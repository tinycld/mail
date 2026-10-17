package mail

import (
	"crypto/sha1"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"text/template"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailhost"
)

var (
	errNoProfileUser    = errors.New("sign in to download the profile")
	errNoProfileAddress = errors.New("no address to set up")
)

// handleMobileconfig serves an Apple configuration profile with the IMAP and
// SMTP settings of the signed-in person's Default From address. It holds no
// password: the device asks for it when the profile is installed.
func handleMobileconfig(app core.App, re *core.RequestEvent) error {
	user, err := profileUser(app, re)
	if err != nil {
		return re.UnauthorizedError("Sign in to download the profile.", nil)
	}
	address, err := profileAddress(app, user)
	if errors.Is(err, errNoProfileAddress) {
		return re.NotFoundError("You have no email address yet.", nil)
	}
	if err != nil {
		return re.InternalServerError("", err)
	}
	// The request reached the web host the app talks to, so that host is the
	// default: the set-up page shows the same one.
	host := mailhost.Host(re.Request.Host)
	if host == "" {
		return re.InternalServerError("", errors.New("no mail host for the profile"))
	}
	body, err := buildMobileconfig(profileInput{
		Email:   address.Email(),
		Name:    profileName(address, user),
		Host:    host,
		OrgName: app.Settings().Meta.AppName,
		UserID:  user.Id,
	})
	if err != nil {
		return re.InternalServerError("", err)
	}
	re.Response.Header().Set("Content-Disposition", `attachment; filename="mail.mobileconfig"`)
	re.Response.Header().Set("Cache-Control", "no-store")
	return re.Blob(http.StatusOK, "application/x-apple-aspen-config", body)
}

// profileUser is the person asking. A browser cannot send a header when it
// opens a link, so a file token in the query also works; an auth token there
// does not, because a link is easy to leak and a file token is short-lived.
func profileUser(app core.App, re *core.RequestEvent) (*core.Record, error) {
	user := re.Auth
	if token := re.Request.URL.Query().Get("token"); token != "" {
		var err error
		user, err = app.FindAuthRecordByToken(token, core.TokenTypeFile)
		if err != nil {
			return nil, err
		}
	}
	if user == nil || user.Collection().Name != "users" {
		return nil, errNoProfileUser
	}
	return user, nil
}

// profileAddress is the address the person sends from by default, chosen as
// a send without a From address chooses it.
func profileAddress(app core.App, user *core.Record) (*resolvedAddress, error) {
	identity, err := app.FindFirstRecordByFilter("mail_identities", "user = {:user} && retired != true", dbx.Params{"user": user.Id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoProfileAddress
	}
	if err != nil {
		return nil, err
	}
	inboxID := identity.GetString("inbox")
	if inboxID == "" {
		return nil, errNoProfileAddress
	}
	inbox, err := app.FindRecordById("mail_mailboxes", inboxID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoProfileAddress
	}
	if err != nil {
		return nil, err
	}
	address, err := defaultSendAddress(app, inbox)
	if errors.Is(err, errNoSendAddress) {
		return nil, errNoProfileAddress
	}
	return address, err
}

// profileName is the sender name mail apps show: the inbox's display name,
// which every send uses, else the person's name, else the address.
func profileName(address *resolvedAddress, user *core.Record) string {
	for _, name := range []string{address.Mailbox.GetString("display_name"), user.GetString("name")} {
		if strings.TrimSpace(name) != "" {
			return name
		}
	}
	return address.Email()
}

type profileInput struct {
	Email, Name, Host, OrgName, UserID string
}

type profileView struct {
	profileInput
	IMAPPort, SMTPPort     int
	DisplayName            string
	ProfileID, ProfileUUID string
	AccountID, AccountUUID string
}

func buildMobileconfig(in profileInput) ([]byte, error) {
	profileID := reverseHost(in.Host) + ".mail." + in.UserID
	accountID := profileID + ".account"
	displayName := "Mail"
	if in.OrgName != "" {
		displayName = in.OrgName + " mail"
	}
	var b strings.Builder
	err := mobileconfigTemplate.Execute(&b, profileView{
		profileInput: in,
		IMAPPort:     clientIMAPPort,
		SMTPPort:     clientSMTPPort,
		DisplayName:  displayName,
		ProfileID:    profileID,
		ProfileUUID:  stableUUID(profileID),
		AccountID:    accountID,
		AccountUUID:  stableUUID(accountID),
	})
	if err != nil {
		return nil, fmt.Errorf("render mobileconfig: %w", err)
	}
	return []byte(b.String()), nil
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// reverseHost gives the reverse-DNS prefix Apple expects for payload ids.
func reverseHost(host string) string {
	parts := strings.Split(host, ".")
	slices.Reverse(parts)
	return strings.Join(parts, ".")
}

// stableUUID derives an RFC 4122 version 5 style UUID from name, so the same
// person gets the same payload ids and a reinstalled profile replaces the old.
func stableUUID(name string) string {
	sum := sha1.Sum([]byte(name))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

var mobileconfigTemplate = template.Must(template.New("mobileconfig").Funcs(template.FuncMap{"x": xmlText}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>EmailAccountDescription</key><string>{{x .Email}}</string>
			<key>EmailAccountName</key><string>{{x .Name}}</string>
			<key>EmailAccountType</key><string>EmailTypeIMAP</string>
			<key>EmailAddress</key><string>{{x .Email}}</string>
			<key>IncomingMailServerAuthentication</key><string>EmailAuthPassword</string>
			<key>IncomingMailServerHostName</key><string>{{x .Host}}</string>
			<key>IncomingMailServerPortNumber</key><integer>{{.IMAPPort}}</integer>
			<key>IncomingMailServerUseSSL</key><true/>
			<key>IncomingMailServerUsername</key><string>{{x .Email}}</string>
			<key>OutgoingMailServerAuthentication</key><string>EmailAuthPassword</string>
			<key>OutgoingMailServerHostName</key><string>{{x .Host}}</string>
			<key>OutgoingMailServerPortNumber</key><integer>{{.SMTPPort}}</integer>
			<key>OutgoingMailServerUseSSL</key><true/>
			<key>OutgoingMailServerUsername</key><string>{{x .Email}}</string>
			<key>OutgoingPasswordSameAsIncomingPassword</key><true/>
			<key>PayloadDisplayName</key><string>{{x .Email}}</string>
			<key>PayloadIdentifier</key><string>{{x .AccountID}}</string>
			<key>PayloadType</key><string>com.apple.mail.managed</string>
			<key>PayloadUUID</key><string>{{.AccountUUID}}</string>
			<key>PayloadVersion</key><integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key><string>Sets up {{x .Email}} in Mail. Mail asks for your password.</string>
	<key>PayloadDisplayName</key><string>{{x .DisplayName}}</string>
	<key>PayloadIdentifier</key><string>{{x .ProfileID}}</string>
	<key>PayloadOrganization</key><string>{{x .OrgName}}</string>
	<key>PayloadRemovalDisallowed</key><false/>
	<key>PayloadType</key><string>Configuration</string>
	<key>PayloadUUID</key><string>{{.ProfileUUID}}</string>
	<key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`))
