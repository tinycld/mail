package mail

import (
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/syscfg"
)

type profileFixture struct {
	app   *tests.TestApp
	user  *core.Record
	guest *core.Record
	com   *core.Record // the person's second address
}

// Same shape as TestLoginIdentifier: nathan@acme.example is the main address,
// nathan@acme.com a second one.
func newProfileFixture(t *testing.T, displayName string) profileFixture {
	t.Helper()
	app := newDiscoveryTestApp(t)
	org, err := app.FindFirstRecordByFilter("mail_domains", "domain = 'acme.example'")
	if err != nil {
		t.Fatal(err)
	}
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "acme.com", "verified": true, "rank": 1})
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "member",
	})
	guest := saveRealRecord(t, app, "users", map[string]any{
		"email": "guest@argosity.com", "username": "guest", "name": "Guest", "role": "guest",
	})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": org.Id, "type": "personal", "display_name": displayName, "name": displayName,
	})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "nathan"})
	second := saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "nathan", "domain": com.Id, "mailbox": inbox.Id, "source": addressSourceManual,
	})
	return profileFixture{app: app, user: user, guest: guest, com: second}
}

func fileToken(t *testing.T, user *core.Record) string {
	t.Helper()
	token, err := user.NewFileToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// runProfile runs s against the fixture and returns the response body and
// headers (ApiScenario checks content, not headers).
func runProfile(t *testing.T, f profileFixture, s tests.ApiScenario) (string, http.Header) {
	t.Helper()
	var body string
	var header http.Header
	s.Method = http.MethodGet
	s.TestAppFactory = func(_ testing.TB) *tests.TestApp { return f.app }
	s.DisableTestAppCleanup = true
	s.AfterTestFunc = func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
		raw, _ := io.ReadAll(res.Body)
		body = string(raw)
		header = res.Header
	}
	s.Test(t)
	return body, header
}

func TestProfileForTheDefaultFromAddress(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	body, header := runProfile(t, f, tests.ApiScenario{
		Name:           "profile by file token",
		URL:            "/api/mail/mobileconfig?token=" + fileToken(t, f.user),
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			"<key>EmailAddress</key><string>nathan@acme.example</string>",
			"<key>IncomingMailServerUsername</key><string>nathan@acme.example</string>",
			"<key>OutgoingMailServerUsername</key><string>nathan@acme.example</string>",
			"<key>IncomingMailServerHostName</key><string>" + discoveryMailHost + "</string>",
			"<key>OutgoingMailServerHostName</key><string>" + discoveryMailHost + "</string>",
			"<key>IncomingMailServerPortNumber</key><integer>993</integer>",
			"<key>OutgoingMailServerPortNumber</key><integer>465</integer>",
			"<key>EmailAccountName</key><string>Nathan</string>",
			"<key>PayloadType</key><string>com.apple.mail.managed</string>",
		},
		NotExpectedContent: []string{"Password</key><string>"},
	})
	assertWellFormedXML(t, body)
	for name, want := range map[string]string{
		"Content-Type":        "application/x-apple-aspen-config",
		"Content-Disposition": `attachment; filename="mail.mobileconfig"`,
		"Cache-Control":       "no-store",
	} {
		if got := header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestProfileFollowsTheChosenDefaultFrom(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	identity, err := f.app.FindFirstRecordByFilter("mail_identities", "user = {:u}", map[string]any{"u": f.user.Id})
	if err != nil {
		t.Fatal(err)
	}
	identity.Set("default_from", f.com.Id)
	if err := f.app.Save(identity); err != nil {
		t.Fatal(err)
	}
	runProfile(t, f, tests.ApiScenario{
		Name:            "chosen default from",
		URL:             "/api/mail/mobileconfig?token=" + fileToken(t, f.user),
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"<key>EmailAddress</key><string>nathan@acme.com</string>"},
	})
}

// unsetMailHost removes the configured mail host, so the profile falls back to
// the public URL or the request.
func unsetMailHost(t *testing.T) {
	t.Helper()
	syscfg.SetResolver(func(string) string { return "" })
}

// A proxy may forward its own address as the Host header; the public URL
// names the host people use, as the set-up page shows.
func TestProfileHostIsThePublicURLNotTheProxyHost(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	unsetMailHost(t)
	f.app.Settings().Meta.AppURL = "https://Web.Acme.example"
	runProfile(t, f, tests.ApiScenario{
		Name:               "bogus Host header",
		URL:                "http://127.0.0.1:8090/api/mail/mobileconfig?token=" + fileToken(t, f.user),
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"<key>IncomingMailServerHostName</key><string>web.acme.example</string>"},
		NotExpectedContent: []string{"127.0.0.1"},
	})
}

func TestProfileHostFallsBackToTheRequestHost(t *testing.T) {
	for name, appURL := range map[string]string{"empty": "", "loopback": "http://localhost:8090"} {
		f := newProfileFixture(t, "Nathan")
		unsetMailHost(t)
		f.app.Settings().Meta.AppURL = appURL
		runProfile(t, f, tests.ApiScenario{
			Name:            "AppURL " + name,
			URL:             "https://web.acme.example/api/mail/mobileconfig?token=" + fileToken(t, f.user),
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{"<key>IncomingMailServerHostName</key><string>web.acme.example</string>"},
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "127.0.0.1": true, "127.9.9.9": true, "::1": true,
		"web.acme.example": false, "10.0.0.1": false, "": false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestProfileByAuthHeader(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	token, err := f.user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	runProfile(t, f, tests.ApiScenario{
		Name:            "auth header",
		URL:             "/api/mail/mobileconfig",
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"nathan@acme.example"},
	})
}

func TestProfileEscapesTheName(t *testing.T) {
	f := newProfileFixture(t, "Ann & <Co>")
	body, _ := runProfile(t, f, tests.ApiScenario{
		Name:            "escaped name",
		URL:             "/api/mail/mobileconfig?token=" + fileToken(t, f.user),
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"<key>EmailAccountName</key><string>Ann &amp; &lt;Co&gt;</string>"},
	})
	assertWellFormedXML(t, body)
}

func TestProfileRefusesWithoutAFileTokenOrSession(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	authToken, err := f.user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	for name, url := range map[string]string{
		"no auth":             "/api/mail/mobileconfig",
		"bad token":           "/api/mail/mobileconfig?token=nope",
		"auth token in query": "/api/mail/mobileconfig?token=" + authToken,
	} {
		runProfile(t, f, tests.ApiScenario{
			Name:               name,
			URL:                url,
			ExpectedStatus:     http.StatusUnauthorized,
			NotExpectedContent: []string{"nathan@acme.example"},
		})
	}
}

func TestProfileForAPersonWithNoAddress(t *testing.T) {
	f := newProfileFixture(t, "Nathan")
	runProfile(t, f, tests.ApiScenario{
		Name:            "guest has no identity",
		URL:             "/api/mail/mobileconfig?token=" + fileToken(t, f.guest),
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{"You have no email address yet."},
	})

	identity, err := f.app.FindFirstRecordByFilter("mail_identities", "user = {:u}", map[string]any{"u": f.user.Id})
	if err != nil {
		t.Fatal(err)
	}
	identity.Set("retired", true)
	identity.Set("inbox", "")
	if err := f.app.Save(identity); err != nil {
		t.Fatal(err)
	}
	runProfile(t, f, tests.ApiScenario{
		Name:            "retired identity",
		URL:             "/api/mail/mobileconfig?token=" + fileToken(t, f.user),
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{"You have no email address yet."},
	})
}

// A reinstalled profile must replace the old one, so its ids are stable.
func TestStableUUID(t *testing.T) {
	a, b := stableUUID("com.example.mail.u1"), stableUUID("com.example.mail.u1")
	if a != b {
		t.Fatalf("stableUUID not deterministic: %s != %s", a, b)
	}
	if a == stableUUID("com.example.mail.u2") {
		t.Fatal("stableUUID collides for different names")
	}
	if !regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-5[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`).MatchString(a) {
		t.Fatalf("stableUUID %q is not an RFC 4122 version 5 UUID", a)
	}
}

func TestReverseHost(t *testing.T) {
	if got := reverseHost("mail.acme.example"); got != "example.acme.mail" {
		t.Fatalf("reverseHost = %q", got)
	}
}
