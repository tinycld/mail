package mail

import (
	"database/sql"
	"encoding/xml"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailhost"
)

// Mail apps connect with implicit TLS on these ports (help: mail:imap and
// mail:smtp). Every answer below and the Apple profile give the same values.
const (
	clientIMAPPort = 993
	clientSMTPPort = 465
)

// autodiscoverBodyLimit bounds an Autodiscover request; a real one is a few
// hundred bytes.
const autodiscoverBodyLimit = 16 << 10

var (
	// Thunderbird asks autoconfig.<domain> first, then the domain's own
	// /.well-known path.
	autoconfigPaths = []string{"/mail/config-v1.1.xml", "/.well-known/autoconfig/mail/config-v1.1.xml"}
	// Outlook uses either case.
	autodiscoverPaths = []string{"/autodiscover/autodiscover.xml", "/Autodiscover/Autodiscover.xml"}
)

// registerClientSetupRoutes serves what mail apps ask for to set themselves
// up: Mozilla autoconfig, Outlook Autodiscover, and the Apple profile. The
// profile route is here because it is the third way a mail app gets its settings.
func registerClientSetupRoutes(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		for _, path := range autoconfigPaths {
			e.Router.GET(path, func(re *core.RequestEvent) error { return handleAutoconfig(app, re) })
		}
		for _, path := range autodiscoverPaths {
			e.Router.POST(path, func(re *core.RequestEvent) error { return handleAutodiscover(app, re) })
		}
		e.Router.GET("/api/mail/mobileconfig", func(re *core.RequestEvent) error { return handleMobileconfig(app, re) })
		return e.Next()
	})
}

// emailDomain returns the lower-case domain of an address, or "".
func emailDomain(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// verifiedMailDomain returns this org's verified domain named name, or nil
// when it has none by that name.
func verifiedMailDomain(app core.App, name string) (*core.Record, error) {
	if name == "" {
		return nil, nil
	}
	row, err := app.FindFirstRecordByFilter("mail_domains", "domain = {:domain} && verified = true", dbx.Params{"domain": name})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// discoveryTarget returns the verified domain a discovery request names and
// the mail host to answer with. Both are "" when there is no answer to give.
// The answer depends on the domain only, so it never tells a caller whether
// an address exists.
func discoveryTarget(app core.App, name string) (domain, host string, err error) {
	row, err := verifiedMailDomain(app, name)
	if err != nil || row == nil {
		return "", "", err
	}
	host = mailhost.Host(app.Settings().Meta.AppURL)
	if host == "" {
		return "", "", nil
	}
	return row.GetString("domain"), host, nil
}

// autoconfigDomain is the domain a Thunderbird request is for: the domain of
// its emailaddress parameter, else the host it was sent to without the
// "autoconfig." label.
func autoconfigDomain(r *http.Request) string {
	if email := strings.TrimSpace(r.URL.Query().Get("emailaddress")); email != "" {
		return emailDomain(email)
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return strings.TrimPrefix(host, "autoconfig.")
}

type autoconfigServer struct {
	Type           string `xml:"type,attr"`
	Hostname       string `xml:"hostname"`
	Port           int    `xml:"port"`
	SocketType     string `xml:"socketType"`
	Authentication string `xml:"authentication"`
	Username       string `xml:"username"`
}

type autoconfigProvider struct {
	ID               string           `xml:"id,attr"`
	Domain           string           `xml:"domain"`
	DisplayName      string           `xml:"displayName"`
	DisplayShortName string           `xml:"displayShortName"`
	Incoming         autoconfigServer `xml:"incomingServer"`
	Outgoing         autoconfigServer `xml:"outgoingServer"`
}

type autoconfigXML struct {
	XMLName  xml.Name           `xml:"clientConfig"`
	Version  string             `xml:"version,attr"`
	Provider autoconfigProvider `xml:"emailProvider"`
}

func autoconfigFor(domain, host, orgName string) autoconfigXML {
	name := orgName
	if name == "" {
		name = domain
	}
	// %EMAILADDRESS% is the address the person typed: the login name is
	// always the full address.
	server := func(kind string, port int) autoconfigServer {
		return autoconfigServer{
			Type: kind, Hostname: host, Port: port, SocketType: "SSL",
			Authentication: "password-cleartext", Username: "%EMAILADDRESS%",
		}
	}
	return autoconfigXML{
		Version: "1.1",
		Provider: autoconfigProvider{
			ID: domain, Domain: domain, DisplayName: name, DisplayShortName: name,
			Incoming: server("imap", clientIMAPPort),
			Outgoing: server("smtp", clientSMTPPort),
		},
	}
}

func handleAutoconfig(app core.App, re *core.RequestEvent) error {
	domain, host, err := discoveryTarget(app, autoconfigDomain(re.Request))
	if err != nil {
		return re.InternalServerError("", err)
	}
	if domain == "" {
		return re.NotFoundError("", nil)
	}
	return re.XML(http.StatusOK, autoconfigFor(domain, host, app.Settings().Meta.AppName))
}

type autodiscoverRequest struct {
	Request struct {
		EMailAddress string `xml:"EMailAddress"`
	} `xml:"Request"`
}

type autodiscoverProtocol struct {
	Type           string `xml:"Type"`
	Server         string `xml:"Server"`
	Port           int    `xml:"Port"`
	LoginName      string `xml:"LoginName"`
	DomainRequired string `xml:"DomainRequired"`
	SPA            string `xml:"SPA"`
	SSL            string `xml:"SSL"`
	Encryption     string `xml:"Encryption,omitempty"`
	AuthRequired   string `xml:"AuthRequired"`
}

type autodiscoverAccount struct {
	AccountType string                 `xml:"AccountType"`
	Action      string                 `xml:"Action"`
	Protocols   []autodiscoverProtocol `xml:"Protocol"`
}

type autodiscoverOutlook struct {
	Account autodiscoverAccount `xml:"Account"`
}

type autodiscoverResponse struct {
	XMLName  xml.Name            `xml:"http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006 Autodiscover"`
	Response autodiscoverOutlook `xml:"http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a Response"`
}

func autodiscoverFor(email, host string) autodiscoverResponse {
	protocol := func(kind string, port int, encryption string) autodiscoverProtocol {
		return autodiscoverProtocol{
			Type: kind, Server: host, Port: port, LoginName: email,
			DomainRequired: "off", SPA: "off", SSL: "on", Encryption: encryption, AuthRequired: "on",
		}
	}
	return autodiscoverResponse{Response: autodiscoverOutlook{Account: autodiscoverAccount{
		AccountType: "email",
		Action:      "settings",
		Protocols: []autodiscoverProtocol{
			protocol("IMAP", clientIMAPPort, ""),
			protocol("SMTP", clientSMTPPort, "SSL"),
		},
	}}}
}

func handleAutodiscover(app core.App, re *core.RequestEvent) error {
	body := http.MaxBytesReader(re.Response, re.Request.Body, autodiscoverBodyLimit)
	var req autodiscoverRequest
	if err := xml.NewDecoder(body).Decode(&req); err != nil {
		return re.BadRequestError("invalid Autodiscover request", nil)
	}
	email := strings.TrimSpace(req.Request.EMailAddress)
	domain, host, err := discoveryTarget(app, emailDomain(email))
	if err != nil {
		return re.InternalServerError("", err)
	}
	if domain == "" {
		return re.NotFoundError("", nil)
	}
	return re.XML(http.StatusOK, autodiscoverFor(email, host))
}
