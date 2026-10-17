package mail

import (
	"crypto/sha1"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"

	"tinycld.org/core/mailhost"
)

var (
	errNoProfileUser    = errors.New("sign in to download the profile")
	errNoProfileAddress = errors.New("no address to set up")
)

// A browser cannot send a header when it opens a link, so the profile link
// carries a token. The token is its own type, so it opens the profile and
// nothing else: PocketBase refuses it as a file token or an auth token, and
// the profile refuses those in turn. It is short-lived because it lands in
// history and logs.
const (
	profileTokenType     = "mailProfile"
	profileTokenDuration = 10 * time.Minute
)

// handleProfileToken gives the signed-in person a token for the profile link.
func handleProfileToken(re *core.RequestEvent) error {
	if re.Auth == nil || re.Auth.Collection().Name != "users" {
		return re.UnauthorizedError("Sign in to download the profile.", nil)
	}
	token, err := newProfileToken(re.Auth, profileTokenDuration)
	if err != nil {
		return re.InternalServerError("", err)
	}
	re.Response.Header().Set("Cache-Control", "no-store")
	return re.JSON(http.StatusOK, map[string]string{"token": token})
}

// profileTokenKey is built as PocketBase builds the file token key, so a
// change of the person's password or token key revokes the token too.
func profileTokenKey(user *core.Record) string {
	return user.TokenKey() + user.Collection().FileToken.Secret
}

func newProfileToken(user *core.Record, duration time.Duration) (string, error) {
	return security.NewJWT(jwt.MapClaims{
		core.TokenClaimType:         profileTokenType,
		core.TokenClaimId:           user.Id,
		core.TokenClaimCollectionId: user.Collection().Id,
	}, profileTokenKey(user), duration)
}

// userByProfileToken returns the person a valid, unexpired profile token names.
func userByProfileToken(app core.App, token string) (*core.Record, error) {
	claims, err := security.ParseUnverifiedJWT(token)
	if err != nil {
		return nil, err
	}
	id, _ := claims[core.TokenClaimId].(string)
	collectionID, _ := claims[core.TokenClaimCollectionId].(string)
	if id == "" || collectionID == "" {
		return nil, errNoProfileUser
	}
	user, err := app.FindRecordById(collectionID, id)
	if err != nil {
		return nil, err
	}
	if user.Collection().Name != "users" {
		return nil, errNoProfileUser
	}
	verified, err := security.ParseJWT(token, profileTokenKey(user))
	if err != nil {
		return nil, err
	}
	if tokenType, _ := verified[core.TokenClaimType].(string); tokenType != profileTokenType {
		return nil, errNoProfileUser
	}
	return user, nil
}

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
	host := profileHost(app, re.Request.Host)
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

// profileHost is the mail host for the profile. The public URL is the default,
// as it is for the set-up page and discovery. A reverse proxy often forwards
// its own address as the Host header, so the request host is only a fallback
// for an install with no public URL set or one that points at this machine.
func profileHost(app core.App, requestHost string) string {
	host := mailhost.Host(app.Settings().Meta.AppURL)
	if host == "" || isLoopbackHost(host) {
		return mailhost.Host(requestHost)
	}
	return host
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// profileUser is the person asking: a profile token in the query, or the
// session. A file token or an auth token in the query does not work, because
// a link is easy to leak.
func profileUser(app core.App, re *core.RequestEvent) (*core.Record, error) {
	user := re.Auth
	if token := re.Request.URL.Query().Get("token"); token != "" {
		var err error
		user, err = userByProfileToken(app, token)
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
