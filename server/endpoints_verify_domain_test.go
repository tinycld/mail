package mail

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/packages/mail/api"
)

// A failed persistence must be visible in the response contract: Saved false
// and SaveError populated. This is the field the UI used to discard, showing
// success while nothing was written.
func TestBuildVerifyDomainResponse_SaveFailure(t *testing.T) {
	app := setupWebhookURLsTestApp(t)
	domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
	record, err := app.FindRecordById("mail_domains", domainID)
	if err != nil {
		t.Fatalf("load domain record: %v", err)
	}

	details := &api.VerificationDetails{ProviderName: "postmark", ProviderConfigured: true}
	body := buildVerifyDomainResponse(record, details, errors.New("disk full"))

	if body.Saved {
		t.Error("Saved = true despite a persistence failure")
	}
	if body.SaveError != "disk full" {
		t.Errorf("SaveError = %q, want the persistence error", body.SaveError)
	}
	if body.ID != record.Id {
		t.Errorf("ID = %q, want %q", body.ID, record.Id)
	}
	if body.VerificationDetails != details {
		t.Error("VerificationDetails not passed through")
	}

	// The wire shape must carry both keys so a typed client can rely on them.
	marshaled, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(marshaled, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire["saved"] != false {
		t.Errorf("wire saved = %v, want false", wire["saved"])
	}
	if wire["save_error"] != "disk full" {
		t.Errorf("wire save_error = %v, want disk full", wire["save_error"])
	}
}

// The success shape omits save_error entirely and reports the record's
// persisted flags.
func TestBuildVerifyDomainResponse_Success(t *testing.T) {
	app := setupWebhookURLsTestApp(t)
	domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
	record, err := app.FindRecordById("mail_domains", domainID)
	if err != nil {
		t.Fatalf("load domain record: %v", err)
	}
	record.Set("verified", true)

	body := buildVerifyDomainResponse(record, &api.VerificationDetails{}, nil)
	if !body.Saved {
		t.Error("Saved = false on success")
	}
	if body.SaveError != "" {
		t.Errorf("SaveError = %q, want empty", body.SaveError)
	}

	marshaled, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(marshaled, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := wire["save_error"]; present {
		t.Error("save_error must be omitted when persistence succeeded")
	}
	if wire["saved"] != true {
		t.Errorf("wire saved = %v, want true", wire["saved"])
	}
}

// Compile-time guard that the fixture record type stays a *core.Record — the
// helper reads flags straight off it.
var _ = func(r *core.Record) { _ = buildVerifyDomainResponse(r, nil, nil) }
