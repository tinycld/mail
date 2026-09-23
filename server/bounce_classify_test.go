package mail

import "testing"

func TestClassifyBounce_ComplaintsWinOverEverything(t *testing.T) {
	// A complaint arrives as its own record type rather than as a bounce, and
	// it is the signal that matters most — reading it as an ordinary bounce
	// would bury it in a bucket with a much looser threshold.
	e := &BounceEvent{RecordType: "SpamComplaint", BounceType: "HardBounce", TypeCode: pmCodeHardBounce}

	got, recognised := classifyBounce(e)
	if !recognised {
		t.Fatal("a spam complaint must be recognised")
	}
	if got != bounceClassComplaint {
		t.Errorf("class = %v, want complaint even when the bounce fields say otherwise", got)
	}
}

func TestClassifyBounce_HardFailures(t *testing.T) {
	for _, code := range []int{pmCodeHardBounce, pmCodeBadEmailAddress, pmCodeBlocked, pmCodeUnconfirmed} {
		got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", TypeCode: code})
		if !recognised {
			t.Errorf("code %d must be recognised", code)
		}
		if got != bounceClassHard {
			t.Errorf("code %d: class = %v, want hard", code, got)
		}
	}
}

// The distinction this whole file exists for. A full mailbox or a greylisting
// server says nothing about the sender, and counting it would judge them for
// who they wrote to.
func TestClassifyBounce_SoftFailuresAreNotTheSendersFault(t *testing.T) {
	for _, code := range []int{pmCodeTransient, pmCodeSoftBounce, pmCodeDNSError, pmCodeSMTPApiError} {
		got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", TypeCode: code})
		if !recognised {
			t.Errorf("code %d must be recognised", code)
		}
		if got != bounceClassSoft {
			t.Errorf("code %d: class = %v, want soft", code, got)
		}
	}
}

func TestClassifyBounce_SpamNotificationIsAComplaint(t *testing.T) {
	// A receiving server's filter saying "this is spam" means the same thing
	// as a human pressing the button; only the reporter differs.
	got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", TypeCode: pmCodeSpamNotification})
	if !recognised || got != bounceClassComplaint {
		t.Errorf("class = %v (recognised=%v), want complaint", got, recognised)
	}
}

// These are not delivery failures at all and must never reach a counter.
func TestClassifyBounce_NonFailuresCountAsNothing(t *testing.T) {
	for _, code := range []int{
		pmCodeUnsubscribe, pmCodeSubscribe, pmCodeAutoResponder,
		pmCodeAddressChange, pmCodeManuallyDeactivated,
	} {
		got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", TypeCode: code})
		if !recognised {
			t.Errorf("code %d must be recognised", code)
		}
		if got != bounceClassNone {
			t.Errorf("code %d: class = %v, want none", code, got)
		}
	}
}

// A DMARC rejection is a configuration problem between the recipient's domain
// and whoever publishes its policy. Counting it would penalise a sender for
// writing to a misconfigured domain.
func TestClassifyBounce_DMARCIsNotTheSendersFault(t *testing.T) {
	got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", TypeCode: pmCodeDMARCPolicy})
	if !recognised {
		t.Fatal("a DMARC rejection must be recognised")
	}
	if got != bounceClassNone {
		t.Errorf("class = %v, want none", got)
	}
}

// A provider that sends a name without a code must still classify, since the
// code is the preferred key but not a guaranteed one.
func TestClassifyBounce_FallsBackToTheDisplayName(t *testing.T) {
	cases := map[string]bounceClass{
		"HardBounce":       bounceClassHard,
		"BadEmailAddress":  bounceClassHard,
		"Blocked":          bounceClassHard,
		"SoftBounce":       bounceClassSoft,
		"Transient":        bounceClassSoft,
		"DnsError":         bounceClassSoft,
		"SpamNotification": bounceClassComplaint,
	}
	for name, want := range cases {
		got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", BounceType: name})
		if !recognised {
			t.Errorf("%q must be recognised from its name alone", name)
		}
		if got != want {
			t.Errorf("%q: class = %v, want %v", name, got, want)
		}
	}
}

// The load-bearing default. A type nobody has mapped must be reported as
// unrecognised rather than guessed at: a new failure type landing silently in
// the hard bucket is a threshold change nobody reviewed.
func TestClassifyBounce_UnknownTypeIsNotGuessedAt(t *testing.T) {
	got, recognised := classifyBounce(&BounceEvent{RecordType: "Bounce", BounceType: "SomethingNew", TypeCode: 999999})
	if recognised {
		t.Error("an unmapped type must report as unrecognised")
	}
	if got != bounceClassNone {
		t.Errorf("class = %v, want none — an unknown failure must not be counted", got)
	}
}

func TestClassifyBounce_EmptyEventIsNotCounted(t *testing.T) {
	if got, recognised := classifyBounce(nil); recognised || got != bounceClassNone {
		t.Errorf("nil event: class = %v (recognised=%v), want none/false", got, recognised)
	}
	if got, recognised := classifyBounce(&BounceEvent{}); recognised || got != bounceClassNone {
		t.Errorf("empty event: class = %v (recognised=%v), want none/false", got, recognised)
	}
}

// The class names reach logs and, later, an operator's dashboard.
func TestBounceClass_String(t *testing.T) {
	cases := map[bounceClass]string{
		bounceClassNone:      "none",
		bounceClassSoft:      "soft",
		bounceClassHard:      "hard",
		bounceClassComplaint: "complaint",
	}
	for class, want := range cases {
		if got := class.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

// The fields the classifier and anything downstream depend on must survive
// the provider's payload. A parser that silently drops From or ID looks fine
// until something tries to attribute or deduplicate.
func TestParseBounce_CarriesTheNewFields(t *testing.T) {
	body := []byte(`{
		"ID": 42,
		"RecordType": "Bounce",
		"Type": "HardBounce",
		"TypeCode": 1,
		"MessageID": "msg-1",
		"From": "sender@example.com",
		"Email": "nobody@example.net",
		"Inactive": true,
		"Description": "The address does not exist",
		"Details": "550 5.1.1 unknown",
		"BouncedAt": "2026-09-22T10:00:00Z"
	}`)

	event, err := NewPostmarkProvider("tok", "acct").ParseBounce(body)
	if err != nil {
		t.Fatalf("ParseBounce: %v", err)
	}

	if event.ID != "42" {
		t.Errorf("ID = %q, want \"42\" — the dedupe key must survive", event.ID)
	}
	if event.From != "sender@example.com" {
		t.Errorf("From = %q — attribution depends on it", event.From)
	}
	if event.TypeCode != 1 {
		t.Errorf("TypeCode = %d, want 1", event.TypeCode)
	}
	if !event.Inactive {
		t.Error("Inactive = false, want true")
	}

	if class, recognised := classifyBounce(event); !recognised || class != bounceClassHard {
		t.Errorf("class = %v (recognised=%v), want hard", class, recognised)
	}
}

// A payload with no ID must not produce a dedupe key of "0", which would
// collide across every such notification.
func TestParseBounce_AbsentIDIsEmptyNotZero(t *testing.T) {
	event, err := NewPostmarkProvider("tok", "acct").
		ParseBounce([]byte(`{"RecordType":"Bounce","MessageID":"m","Type":"HardBounce"}`))
	if err != nil {
		t.Fatalf("ParseBounce: %v", err)
	}
	if event.ID != "" {
		t.Errorf("ID = %q, want empty — a missing id must not become a shared key", event.ID)
	}
}
