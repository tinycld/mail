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
		handler := carddav.Handler{Backend: backend}

		serveCardDAV := func(re *core.RequestEvent) error {
			// Inject the raw HTTP request into context for auth extraction
			ctx := context.WithValue(re.Request.Context(), httpRequestKey, re.Request)
			handler.ServeHTTP(re.Response, re.Request.WithContext(ctx))
			return nil
		}

		e.Router.Any("/carddav/{path...}", serveCardDAV)
		e.Router.Any("/carddav", serveCardDAV)

		e.Router.GET("/.well-known/carddav", func(re *core.RequestEvent) error {
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
