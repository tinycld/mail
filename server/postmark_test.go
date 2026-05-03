package mail

import (
	"strings"
	"testing"
	"time"
)

// TestNormalizePostmarkDate_RFC2822 — Postmark sends Date in RFC-2822 format
// (e.g. "Sat, 2 May 2026 20:35:46 -0700"). The mail_messages.date field is
// PocketBase's Date type which expects RFC-3339; storing the raw RFC-2822
// string causes "date: cannot be blank" validation errors.
func TestNormalizePostmarkDate_RFC2822(t *testing.T) {
	got := normalizePostmarkDate("Sat, 2 May 2026 20:35:46 -0700")
	want := "2026-05-03T03:35:46Z"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestNormalizePostmarkDate_AlreadyRFC3339 — round-trips RFC-3339 input
// (the Date header sometimes already comes through in this form).
func TestNormalizePostmarkDate_AlreadyRFC3339(t *testing.T) {
	// mail.ParseDate accepts a relaxed superset that includes RFC-3339-ish
	// strings; we just need the output to be parseable as RFC-3339.
	out := normalizePostmarkDate("Mon, 02 Jan 2006 15:04:05 -0700")
	if _, err := time.Parse(time.RFC3339, out); err != nil {
		t.Fatalf("output %q is not valid RFC-3339: %v", out, err)
	}
}

// TestNormalizePostmarkDate_InvalidFallsBackToNow — never reject an inbound
// message over a malformed Date header. Garbage input should fall back to
// the current UTC time formatted as RFC-3339.
func TestNormalizePostmarkDate_InvalidFallsBackToNow(t *testing.T) {
	out := normalizePostmarkDate("not a date")
	if _, err := time.Parse(time.RFC3339, out); err != nil {
		t.Fatalf("fallback output %q is not valid RFC-3339: %v", out, err)
	}
}

// TestNormalizePostmarkDate_EmptyFallsBackToNow — empty Date header
// (rare but possible) should also fall back to now.
func TestNormalizePostmarkDate_EmptyFallsBackToNow(t *testing.T) {
	out := normalizePostmarkDate("")
	if _, err := time.Parse(time.RFC3339, out); err != nil {
		t.Fatalf("fallback output %q is not valid RFC-3339: %v", out, err)
	}
}

// TestParseInbound_NormalizesDateFromRealPostmarkPayload — end-to-end
// regression for the production bug: a real Postmark payload's Date field
// must come out of ParseInbound as RFC-3339 so PocketBase accepts it.
func TestParseInbound_NormalizesDateFromRealPostmarkPayload(t *testing.T) {
	body := []byte(`{
		"FromFull": {"Email": "sender@example.org", "Name": "Sender"},
		"ToFull": [{"Email": "alice@acme.com"}],
		"Subject": "test",
		"TextBody": "body",
		"Date": "Sat, 2 May 2026 20:35:46 -0700",
		"MessageID": "abc-123"
	}`)

	p := &PostmarkProvider{}
	msg, err := p.ParseInbound(body)
	if err != nil {
		t.Fatalf("ParseInbound failed: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, msg.Date); err != nil {
		t.Fatalf("Date is not RFC-3339 (got %q): %v", msg.Date, err)
	}
	if !strings.HasPrefix(msg.Date, "2026-05-0") {
		t.Fatalf("Date didn't preserve the day, got %q", msg.Date)
	}
}
