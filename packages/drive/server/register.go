package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/emersion/go-webdav"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

func Register(app *pocketbase.PocketBase) {
	// FTS sync hooks for drive_items
	app.OnRecordAfterCreateSuccess("drive_items").BindFunc(func(e *core.RecordEvent) error {
		syncDriveItemToFTS(app, e.Record, "create")
		go extractAndIndexDriveItem(app, e.Record)
		go generateThumbnail(app, e.Record)
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("drive_items").BindFunc(func(e *core.RecordEvent) error {
		syncDriveItemToFTS(app, e.Record, "update")
		go extractAndIndexDriveItem(app, e.Record)
		go generateThumbnail(app, e.Record)
		return e.Next()
	})

	app.OnRecordAfterDeleteSuccess("drive_items").BindFunc(func(e *core.RecordEvent) error {
		syncDriveItemToFTS(app, e.Record, "delete")
		return e.Next()
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Search API endpoint
		e.Router.GET("/api/drive/search", func(re *core.RequestEvent) error {
			return handleDriveSearch(app, re)
		}).BindFunc(requireAuth)

		// Share endpoint (creates shares + sends invite emails)
		e.Router.POST("/api/drive/share", func(re *core.RequestEvent) error {
			return handleShare(app, re)
		}).BindFunc(requireAuth)

		// Version history endpoints
		e.Router.POST("/api/drive/upload-version", func(re *core.RequestEvent) error {
			return handleUploadVersion(app, re)
		}).BindFunc(requireAuth)

		e.Router.POST("/api/drive/versions/restore", func(re *core.RequestEvent) error {
			return handleRestoreVersion(app, re)
		}).BindFunc(requireAuth)

		// WebDAV handler
		filesystem := &DriveFileSystem{app: app}
		handler := &webdav.Handler{FileSystem: filesystem}

		serveWebDAV := func(re *core.RequestEvent) error {
			_, _, ok := re.Request.BasicAuth()
			if !ok {
				re.Response.Header().Set("WWW-Authenticate", `Basic realm="TinyCld WebDAV"`)
				http.Error(re.Response, "Authentication required", http.StatusUnauthorized)
				return nil
			}

			ctx := context.WithValue(re.Request.Context(), httpRequestKey, re.Request)
			handler.ServeHTTP(re.Response, re.Request.WithContext(ctx))
			return nil
		}

		e.Router.Any("/webdav/{path...}", serveWebDAV)
		e.Router.Any("/webdav", serveWebDAV)

		e.Router.Any("/.well-known/webdav", func(re *core.RequestEvent) error {
			http.Redirect(re.Response, re.Request, "/webdav/", http.StatusMovedPermanently)
			return nil
		})

		return e.Next()
	})
}

func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}

// resolveItemAndUserOrg loads the item, validates the user has an org membership matching
// the item's org, and returns the item plus the matching user_org ID.
// If requireWrite is true, also validates editor/owner share permission.
func resolveItemAndUserOrg(app *pocketbase.PocketBase, re *core.RequestEvent, itemID string, requireWrite bool) (*core.Record, string, error) {
	item, err := app.FindRecordById("drive_items", itemID)
	if err != nil {
		return nil, "", re.NotFoundError("item not found", nil)
	}

	itemOrgID := item.GetString("org")

	userOrgIDs, err := getUserOrgIDs(app, re.Auth.Id)
	if err != nil || len(userOrgIDs) == 0 {
		return nil, "", re.ForbiddenError("no access", nil)
	}

	// Build a set of user_org IDs that belong to the item's org
	orgUserOrgs, err := app.FindRecordsByFilter(
		"user_org",
		"user = {:user} && org = {:org}",
		"", 1, 0,
		map[string]any{"user": re.Auth.Id, "org": itemOrgID},
	)
	if err != nil || len(orgUserOrgs) == 0 {
		return nil, "", re.ForbiddenError("no org membership for this item", nil)
	}

	matchedUserOrgID := orgUserOrgs[0].Id

	if requireWrite {
		if err := checkWritePermission(app, matchedUserOrgID, item.Id); err != nil {
			return nil, "", re.ForbiddenError("editor or owner access required", nil)
		}
	} else {
		shares, err := app.FindRecordsByFilter(
			"drive_shares",
			"item = {:item} && user_org = {:uo}",
			"", 1, 0,
			map[string]any{"item": item.Id, "uo": matchedUserOrgID},
		)
		if err != nil || len(shares) == 0 {
			return nil, "", re.ForbiddenError("no access to item", nil)
		}
	}

	return item, matchedUserOrgID, nil
}

// handleUploadVersion snapshots the current file and replaces it with the uploaded one.
func handleUploadVersion(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	itemID := re.Request.FormValue("item")
	if itemID == "" {
		return re.BadRequestError("missing item parameter", nil)
	}

	item, userOrgID, err := resolveItemAndUserOrg(app, re, itemID, true)
	if err != nil {
		return err
	}

	file, header, err := re.Request.FormFile("file")
	if err != nil {
		return re.BadRequestError("missing file", nil)
	}
	defer file.Close()

	if err := snapshotCurrentFile(app, item, userOrgID, "upload", ""); err != nil {
		app.Logger().Warn("version snapshot failed during upload", "id", item.Id, "error", err)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return re.BadRequestError("failed to read file", nil)
	}

	f, err := filesystem.NewFileFromBytes(data, header.Filename)
	if err != nil {
		return re.BadRequestError("failed to create file", nil)
	}

	item.Set("file", f)
	item.Set("size", len(data))
	item.Set("mime_type", header.Header.Get("Content-Type"))

	if err := app.Save(item); err != nil {
		return re.BadRequestError("failed to save item", nil)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"id":        item.Id,
		"name":      item.GetString("name"),
		"file":      item.GetString("file"),
		"size":      item.GetInt("size"),
		"mime_type": item.GetString("mime_type"),
	})
}

// handleRestoreVersion restores a previous version as the current file.
func handleRestoreVersion(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		Item    string `json:"item"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("invalid request body", nil)
	}

	if body.Item == "" || body.Version == "" {
		return re.BadRequestError("missing item or version", nil)
	}

	item, userOrgID, err := resolveItemAndUserOrg(app, re, body.Item, true)
	if err != nil {
		return err
	}

	version, err := app.FindRecordById("drive_item_versions", body.Version)
	if err != nil {
		return re.NotFoundError("version not found", nil)
	}

	if version.GetString("item") != item.Id {
		return re.BadRequestError("version does not belong to item", nil)
	}

	// Snapshot the current file before restoring (system-generated, hidden from UI)
	if err := snapshotCurrentFile(app, item, userOrgID, "system", ""); err != nil {
		app.Logger().Warn("version snapshot failed during restore", "id", item.Id, "error", err)
	}

	versionFilename := version.GetString("file")
	if versionFilename == "" {
		return re.BadRequestError("version has no file", nil)
	}

	fsys, err := app.NewFilesystem()
	if err != nil {
		return re.BadRequestError("filesystem error", nil)
	}
	defer fsys.Close()

	key := version.BaseFilesPath() + "/" + versionFilename
	reader, err := fsys.GetReader(key)
	if err != nil {
		return re.BadRequestError("failed to read version file", nil)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return re.BadRequestError("failed to read version data", nil)
	}

	f, err := filesystem.NewFileFromBytes(data, versionFilename)
	if err != nil {
		return re.BadRequestError("failed to create file from version", nil)
	}

	item.Set("file", f)
	item.Set("size", version.GetInt("size"))
	item.Set("mime_type", version.GetString("mime_type"))

	if err := app.Save(item); err != nil {
		return re.BadRequestError("failed to save restored item", nil)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"id":        item.Id,
		"name":      item.GetString("name"),
		"file":      item.GetString("file"),
		"size":      item.GetInt("size"),
		"mime_type": item.GetString("mime_type"),
	})
}
