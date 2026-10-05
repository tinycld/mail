package mail

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"tinycld.org/packages/mail/api"
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

func TestMapResults_PropagatesStateID(t *testing.T) {
	rows := []searchResultRow{
		{ThreadID: "t1", StateID: "state1"},
		{ThreadID: "t2", StateID: "state2"},
	}

	items := mapResults(rows)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].StateID != "state1" {
		t.Errorf("items[0].StateID = %q, want state1", items[0].StateID)
	}
	if items[1].StateID != "state2" {
		t.Errorf("items[1].StateID = %q, want state2", items[1].StateID)
	}
}

// Every UNION ALL arm of the FTS search must carry the same columns. When
// state_id joined the thread and message arms, the placeholder arm kept the
// old shape, and every live search failed with "SELECTs to the left and right
// of UNION ALL do not have the same number of result columns". The fixture
// above has no FTS tables, so this runs the query against real ones.
func TestSearchMail_FTSQueryRunsAgainstLiveShapedIndexes(t *testing.T) {
	app := setupFTSMatchApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_searchfts_01")
	user := searchUser(t, app, "alice@acme.com")
	seedMember(t, app, "mb_searchfts_01", user.Id)

	for name, req := range map[string]api.SearchRequest{
		"query":     {Query: "alpha", Limit: 25},
		"body only": {HasWords: "alpha", Limit: 25},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := SearchMail(app, user.Id, req); err != nil {
				t.Fatalf("search failed: %v", err)
			}
		})
	}
}
