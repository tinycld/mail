package mail

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// A database failure while looking up an address is not "no such address":
// treating it as one bounces or drops mail that a retry would deliver. Each
// test breaks mail_addresses, the table every lookup reads.

func dropAddressTable(t *testing.T, app core.App) {
	t.Helper()
	if _, err := app.DB().NewQuery("DROP TABLE mail_addresses").Execute(); err != nil {
		t.Fatal(err)
	}
}

func lookupFailureMessage(id string) *InboundMessage {
	return &InboundMessage{
		From:      Recipient{Email: "sender@sender.example"},
		To:        []Recipient{{Email: "nathan@example.org"}},
		Subject:   "lookup failure",
		TextBody:  "body",
		MessageID: "<" + id + "@sender.example>",
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
}

func TestResolveAddress_DatabaseErrorIsNotUnknownAddress(t *testing.T) {
	env := setupRoutingEnv(t)
	if _, err := resolveAddress(env.app, "nathan", "nowhere.example"); !errors.Is(err, errAddressNotFound) {
		t.Fatalf("unknown domain: want errAddressNotFound, got %v", err)
	}
	dropAddressTable(t, env.app)
	_, err := resolveAddress(env.app, "nathan", "example.org")
	if err == nil || errors.Is(err, errAddressNotFound) {
		t.Fatalf("broken address table: want a database error, got %v", err)
	}

	if _, err := env.app.DB().NewQuery("DROP TABLE mail_domains").Execute(); err != nil {
		t.Fatal(err)
	}
	_, err = resolveAddress(env.app, "nathan", "example.org")
	if err == nil || errors.Is(err, errAddressNotFound) {
		t.Fatalf("broken domain table: want a database error, got %v", err)
	}
}

func TestRouteInbound_LookupFailureIsAStorageFailure(t *testing.T) {
	env := setupRoutingEnv(t)
	dropAddressTable(t, env.app)
	result := routeInbound(env.app, lookupFailureMessage("route-fail"))
	if len(result.storageFailures) != 1 || len(result.unknownAddrs) != 0 {
		t.Fatalf("storage failures %v, unknown %v; want one failure and no unknown address",
			result.storageFailures, result.unknownAddrs)
	}
}

// The webhook answers 500 so the provider retries, instead of 200 "unknown
// recipient", which drops the message.
func TestHandleInbound_AddressLookupFailureReturns500(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_lookfail_001")
	seedMember(t, app, "mb_lookfail_001", "user_alice")
	dropAddressTable(t, app)

	body := postmarkPayload(t, []string{"alice@acme.com"}, "lookfail", "body", "<msg-lookfail-1@example.org>")
	re, _ := makeInboundRequest(t, app, "tok-lookfail", body)
	err := handleInbound(app, &stubProvider{parse: (&PostmarkProvider{}).ParseInbound}, re, "tok-lookfail")
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError {
		t.Fatalf("want a 500, got %v", err)
	}
}

func TestSMTPInboundRcpt_LookupFailureIsTemporary(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_lookfail_002")
	dropAddressTable(t, app)

	sess := &smtpInboundSession{app: app, hostname: "mx.tinycld.test"}
	if err := sess.Mail("sender@external.example", &smtp.MailOptions{}); err != nil {
		t.Fatal(err)
	}
	err := sess.Rcpt("alice@acme.com", &smtp.RcptOptions{})
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 451 || smtpErr.EnhancedCode != (smtp.EnhancedCode{4, 3, 0}) {
		t.Fatalf("want 451 4.3.0, got %v", err)
	}
}

// The IMAP fetcher leaves a message unseen when dispatch fails, so it is
// fetched again on the next tick.
func TestDispatchInbound_LookupFailureIsReturned(t *testing.T) {
	env := setupRoutingEnv(t)
	dropAddressTable(t, env.app)
	if err := dispatchInbound(env.app, lookupFailureMessage("dispatch-fail")); err == nil {
		t.Fatal("dispatchInbound hid a lookup failure; the message would be marked done and lost")
	}
}

func TestSendAddress_LookupFailureIsAnInternalError(t *testing.T) {
	env := setupSendAddressEnv(t)
	row, _ := findAddressRow(env.app, "nathan", env.com.Id)
	alias := saveRealRecord(t, env.app, "mail_mailbox_aliases", map[string]any{"mailbox": env.inbox.Id, "address": "admin"})
	dropAddressTable(t, env.app)

	for name, call := range map[string]func() error{
		"from_address_id": func() error { _, err := resolveSendAddress(env.app, env.inbox, row.Id, ""); return err },
		"alias_id":        func() error { _, err := resolveSendAddress(env.app, env.inbox, "", alias.Id); return err },
	} {
		err := call()
		if err == nil || errors.Is(err, errAddressNotFound) {
			t.Fatalf("%s: want a database error, got %v", name, err)
		}
		if kind := sendErrorForAddress(err).kind; kind != sendErrInternal {
			t.Fatalf("%s: send error kind = %v, want internal", name, kind)
		}
	}
}

func TestSMTPSubmissionMail_LookupFailureIsTemporary(t *testing.T) {
	env, s := smtpSendEnv(t)
	dropAddressTable(t, env.app)
	err := s.Mail("nathan@tinycld.com", nil)
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 451 {
		t.Fatalf("want 451, got %v", err)
	}
}
