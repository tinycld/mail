package contacts

import (
	"context"
	"net/http"

	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func Register(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		backend := &CardDAVBackend{app: app}
		handler := carddav.Handler{Backend: backend, Prefix: "/carddav"}

		serveCardDAV := func(re *core.RequestEvent) error {
			// Require Basic Auth — send 401 challenge if missing
			_, _, ok := re.Request.BasicAuth()
			if !ok {
				re.Response.Header().Set("WWW-Authenticate", `Basic realm="TinyCld CardDAV"`)
				http.Error(re.Response, "Authentication required", http.StatusUnauthorized)
				return nil
			}

			ctx := context.WithValue(re.Request.Context(), httpRequestKey, re.Request)
			handler.ServeHTTP(re.Response, re.Request.WithContext(ctx))
			return nil
		}

		e.Router.Any("/carddav/{path...}", serveCardDAV)
		e.Router.Any("/carddav", serveCardDAV)

		e.Router.Any("/.well-known/carddav", func(re *core.RequestEvent) error {
			http.Redirect(re.Response, re.Request, "/carddav/", http.StatusMovedPermanently)
			return nil
		})

		return e.Next()
	})

	// Auto-generate vcard_uid for contacts created via the web UI
	app.OnRecordCreate("contacts").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("vcard_uid") == "" {
			e.Record.Set("vcard_uid", "urn:uuid:"+uuid.NewString())
		}
		return e.Next()
	})
}
