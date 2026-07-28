package mail

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// searchUser creates an auth record the search handler can run as.
func searchUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// A failed search query must be an error the client can see, not
// HTTP 200 {"items":[],"total":0}. That swallow is exactly what let the
// `ts.user_org` rename bug present as a silent empty inbox: the whole Go
// suite stayed green while every live search returned nothing.
func TestHandleSearch_QueryFailureIsAnError(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_searcherr_01")
	user := searchUser(t, app, "alice@acme.com")
	seedMember(t, app, "mb_searcherr_01", user.Id)

	// The fixture app has no FTS tables, so the search SQL fails — the same
	// class of failure (schema drift, bad SQL) the swallow used to hide.
	req := httptest.NewRequest(http.MethodGet, "/api/mail/search?q=hello", nil)
	rec := httptest.NewRecorder()
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user

	err := handleSearch(app, re)
	if err == nil {
		t.Fatalf("search over a broken schema reported success; body=%s", rec.Body.String())
	}
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError {
		t.Fatalf("expected a 500 ApiError, got %T: %v", err, err)
	}
}

func TestMapResults_PropagatesHasAttachments(t *testing.T) {
	rows := []searchResultRow{
		{ThreadID: "t1", HasAttachments: true},
		{ThreadID: "t2", HasAttachments: false},
	}

	items := mapResults(rows)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].HasAttachments {
		t.Errorf("expected items[0].HasAttachments=true, got false")
	}
	if items[1].HasAttachments {
		t.Errorf("expected items[1].HasAttachments=false, got true")
	}
}
