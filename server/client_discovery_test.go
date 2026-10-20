package mail

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/mailhost"
	"tinycld.org/core/syscfg"
)

const discoveryMailHost = "mail.acme.example"

func newDiscoveryTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	syscfg.ResetForTesting()
	t.Cleanup(syscfg.ResetForTesting)
	syscfg.SetResolver(func(key string) string {
		if key == mailhost.Key {
			return discoveryMailHost
		}
		return ""
	})
	app := newAddressTestApp(t)
	app.Settings().Meta.AppName = "Acme & Co"
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "acme.example", "verified": true})
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "pending.example", "verified": false})
	registerClientSetupRoutes(app)
	return app
}

// seedAnn creates a real person with the address ann@acme.example, so a test
// can compare a real address with an unknown one.
func seedAnn(t *testing.T, app *tests.TestApp) {
	t.Helper()
	org, err := app.FindFirstRecordByFilter("mail_domains", "domain = 'acme.example'")
	if err != nil {
		t.Fatal(err)
	}
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "ann@argosity.com", "username": "ann", "name": "Ann", "role": "member",
	})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "ann", "domain": org.Id, "type": "personal", "display_name": "Ann", "name": "Ann",
	})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "ann"})
	if _, err := app.FindFirstRecordByFilter("mail_addresses", "local_part = 'ann' && mailbox = {:m}", map[string]any{"m": inbox.Id}); err != nil {
		t.Fatalf("ann@acme.example is not a real address: %v", err)
	}
}

func runDiscovery(t *testing.T, app *tests.TestApp, s tests.ApiScenario) {
	t.Helper()
	s.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	s.DisableTestAppCleanup = true
	s.Test(t)
}

func assertWellFormedXML(t testing.TB, body string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(body))
	dec.Strict = true
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, body)
		}
	}
}

func TestAutoconfigAnswersForAVerifiedDomain(t *testing.T) {
	app := newDiscoveryTestApp(t)
	for _, path := range autoconfigPaths {
		runDiscovery(t, app, tests.ApiScenario{
			Name:           "autoconfig " + path,
			Method:         http.MethodGet,
			URL:            path + "?emailaddress=Ann@ACME.example",
			ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{
				`<emailProvider id="acme.example">`,
				`<incomingServer type="imap">`,
				"<hostname>" + discoveryMailHost + "</hostname>",
				"<port>993</port>",
				"<port>465</port>",
				"<socketType>SSL</socketType>",
				"<username>%EMAILADDRESS%</username>",
				"<displayName>Acme &amp; Co</displayName>",
			},
			AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
				body, _ := io.ReadAll(res.Body)
				assertWellFormedXML(t, string(body))
			},
		})
	}
}

func TestAutoconfigRefusesOtherDomains(t *testing.T) {
	app := newDiscoveryTestApp(t)
	for _, q := range []string{
		"?emailaddress=ann@pending.example", // owned, not verified
		"?emailaddress=ann@other.example",   // not owned
		"?emailaddress=not-an-address",
		"?emailaddress=ann@",
	} {
		runDiscovery(t, app, tests.ApiScenario{
			Name:               "autoconfig refuses " + q,
			Method:             http.MethodGet,
			URL:                "/mail/config-v1.1.xml" + q,
			ExpectedStatus:     http.StatusNotFound,
			NotExpectedContent: []string{discoveryMailHost},
		})
	}
}

// The answer depends on the domain only, so it does not tell a caller which
// addresses exist.
func TestAutoconfigGivesTheSameAnswerForEveryLocalPart(t *testing.T) {
	app := newDiscoveryTestApp(t)
	seedAnn(t, app)
	var bodies []string
	for _, email := range []string{"ann@acme.example", "nobody@acme.example"} {
		runDiscovery(t, app, tests.ApiScenario{
			Name:            "autoconfig " + email,
			Method:          http.MethodGet,
			URL:             "/mail/config-v1.1.xml?emailaddress=" + email,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{"<clientConfig"},
			AfterTestFunc: func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
				body, _ := io.ReadAll(res.Body)
				bodies = append(bodies, string(body))
			},
		})
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("answers differ by local part:\n%v", bodies)
	}
}

func TestAutoconfigDomainFromTheRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ target, want string }{
		{"http://autoconfig.acme.example/mail/config-v1.1.xml", "acme.example"},
		{"http://AutoConfig.Acme.Example:443/mail/config-v1.1.xml", "acme.example"},
		{"http://acme.example/.well-known/autoconfig/mail/config-v1.1.xml", "acme.example"},
		{"http://autoconfig.acme.example/mail/config-v1.1.xml?emailaddress=ann@other.example", "other.example"},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.target, nil)
		if got := autoconfigDomain(r); got != tc.want {
			t.Errorf("autoconfigDomain(%s) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestEmailDomain(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"ann@acme.example", "acme.example"},
		{" Ann@ACME.Example ", "acme.example"},
		{"a@b@acme.example", "acme.example"},
		{"ann", ""},
		{"@acme.example", ""},
		{"ann@", ""},
		{"", ""},
	} {
		if got := emailDomain(tc.in); got != tc.want {
			t.Errorf("emailDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func autodiscoverBody(email string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006">
  <Request>
    <EMailAddress>` + email + `</EMailAddress>
    <AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema>
  </Request>
</Autodiscover>`
}

func TestAutodiscoverAnswersForAVerifiedDomain(t *testing.T) {
	app := newDiscoveryTestApp(t)
	for _, path := range autodiscoverPaths {
		runDiscovery(t, app, tests.ApiScenario{
			Name:           "autodiscover " + path,
			Method:         http.MethodPost,
			URL:            path,
			Body:           strings.NewReader(autodiscoverBody("ann&amp;bo@acme.example")),
			ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{
				`xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006"`,
				`xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a"`,
				"<Type>IMAP</Type>",
				"<Type>SMTP</Type>",
				"<Server>" + discoveryMailHost + "</Server>",
				"<Port>993</Port>",
				"<Port>465</Port>",
				"<SSL>on</SSL>",
				"<LoginName>ann&amp;bo@acme.example</LoginName>",
			},
			AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
				body, _ := io.ReadAll(res.Body)
				assertWellFormedXML(t, string(body))
			},
		})
	}
}

// A real address and an unknown one differ only in the LoginName the caller
// sent, so the answer does not tell a caller which addresses exist.
func TestAutodiscoverGivesTheSameAnswerForEveryLocalPart(t *testing.T) {
	app := newDiscoveryTestApp(t)
	seedAnn(t, app)
	var bodies []string
	for _, email := range []string{"ann@acme.example", "nobody@acme.example"} {
		runDiscovery(t, app, tests.ApiScenario{
			Name:            "autodiscover " + email,
			Method:          http.MethodPost,
			URL:             "/autodiscover/autodiscover.xml",
			Body:            strings.NewReader(autodiscoverBody(email)),
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{"<LoginName>" + email + "</LoginName>"},
			AfterTestFunc: func(_ testing.TB, _ *tests.TestApp, res *http.Response) {
				body, _ := io.ReadAll(res.Body)
				bodies = append(bodies, strings.ReplaceAll(string(body), email, "ADDRESS"))
			},
		})
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("answers differ by local part:\n%v", bodies)
	}
}

func TestAutodiscoverRefusesBadRequests(t *testing.T) {
	app := newDiscoveryTestApp(t)
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"unverified domain", autodiscoverBody("ann@pending.example"), http.StatusNotFound},
		{"other domain", autodiscoverBody("ann@other.example"), http.StatusNotFound},
		{"no address", autodiscoverBody(""), http.StatusNotFound},
		{"not XML", "hello", http.StatusBadRequest},
		{"too large", autodiscoverBody(strings.Repeat("a", autodiscoverBodyLimit) + "@acme.example"), http.StatusBadRequest},
	} {
		runDiscovery(t, app, tests.ApiScenario{
			Name:               "autodiscover " + tc.name,
			Method:             http.MethodPost,
			URL:                "/autodiscover/autodiscover.xml",
			Body:               strings.NewReader(tc.body),
			ExpectedStatus:     tc.status,
			NotExpectedContent: []string{discoveryMailHost},
		})
	}
}
