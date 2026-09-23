package mail

import "strings"

// bounceClass is what a delivery failure MEANS, as opposed to what it is
// called or how it is displayed.
//
// The distinction exists because only some failures say anything about the
// sender. An address that does not exist says the list was not earned; a
// mailbox that is full says nothing at all. Anything that counts failures in
// order to judge a sender has to tell those apart, and the provider's own
// vocabulary does not — it has a dozen type names across three meanings.
type bounceClass int

const (
	// bounceClassNone is a notification that is not a delivery failure at
	// all: an auto-responder, an unsubscribe, an address change. Never
	// counted.
	bounceClassNone bounceClass = iota

	// bounceClassSoft is a temporary failure — a full mailbox, greylisting, a
	// DNS blip, the receiving server having a bad afternoon. This is the
	// recipient's problem, not the sender's, and counting it would judge a
	// sender for who they wrote to.
	bounceClassSoft

	// bounceClassHard is a permanent failure: the address does not exist, or
	// the receiving server refuses it outright. One is an accident. A high
	// proportion of them means the list was never opt-in.
	bounceClassHard

	// bounceClassComplaint is a recipient, or a receiving server acting for
	// one, saying this was spam. The strongest signal available: there is no
	// innocent explanation for it at volume, and it is the one that costs a
	// sending reputation most.
	bounceClassComplaint
)

func (c bounceClass) String() string {
	switch c {
	case bounceClassSoft:
		return "soft"
	case bounceClassHard:
		return "hard"
	case bounceClassComplaint:
		return "complaint"
	default:
		return "none"
	}
}

// Postmark's stable numeric type codes. The display names beside them change;
// the codes do not, which is why these are the primary key for classification
// and the name is only a fallback.
const (
	pmCodeHardBounce          = 1
	pmCodeTransient           = 2
	pmCodeUnsubscribe         = 16
	pmCodeSubscribe           = 32
	pmCodeAutoResponder       = 64
	pmCodeAddressChange       = 128
	pmCodeDNSError            = 256
	pmCodeSpamNotification    = 512
	pmCodeOpenRelayTest       = 1024
	pmCodeUnknown             = 2048
	pmCodeSoftBounce          = 4096
	pmCodeVirusNotification   = 8192
	pmCodeChallengeVerif      = 16384
	pmCodeBadEmailAddress     = 100000
	pmCodeSpamComplaint       = 100001
	pmCodeManuallyDeactivated = 100002
	pmCodeUnconfirmed         = 100003
	pmCodeBlocked             = 100006
	pmCodeSMTPApiError        = 100007
	pmCodeInboundError        = 100008
	pmCodeDMARCPolicy         = 100009
	pmCodeTemplateRendering   = 100010
)

// classifyBounce decides what a delivery notification means.
//
// RecordType is checked first because a spam complaint arrives as its own
// record type rather than as a bounce, and it is the signal that matters
// most — misreading one as an ordinary bounce would bury it.
//
// Then the numeric code, then the display name as a fallback for a provider
// that sends one without the other.
//
// An unrecognised failure returns bounceClassNone and is reported as
// unrecognised, deliberately. Providers add types, and a new one landing
// silently in the hard-bounce bucket would be a threshold change nobody
// reviewed — the counters would tighten on their own. Not counting it risks
// missing some abuse; guessing risks throttling someone who did nothing, and
// of the two, the one that needs a human to notice is the safer failure.
func classifyBounce(e *BounceEvent) (class bounceClass, recognised bool) {
	if e == nil {
		return bounceClassNone, false
	}

	if strings.EqualFold(e.RecordType, "SpamComplaint") {
		return bounceClassComplaint, true
	}

	switch e.TypeCode {
	case pmCodeSpamComplaint, pmCodeSpamNotification, pmCodeVirusNotification:
		return bounceClassComplaint, true
	case pmCodeHardBounce, pmCodeBadEmailAddress, pmCodeBlocked, pmCodeUnconfirmed:
		return bounceClassHard, true
	case pmCodeTransient, pmCodeSoftBounce, pmCodeDNSError, pmCodeSMTPApiError,
		pmCodeInboundError, pmCodeTemplateRendering:
		return bounceClassSoft, true
	case pmCodeUnsubscribe, pmCodeSubscribe, pmCodeAutoResponder,
		pmCodeAddressChange, pmCodeManuallyDeactivated, pmCodeOpenRelayTest,
		pmCodeChallengeVerif, pmCodeUnknown, pmCodeDMARCPolicy:
		// Not delivery failures, or failures the sender did not cause.
		//
		// DMARCPolicy is the interesting one: it means the RECIPIENT's domain
		// published a policy that rejected the message. That is a
		// configuration problem between two other parties, and counting it
		// would penalise a sender for writing to a misconfigured domain.
		return bounceClassNone, true
	}

	// No code, or one we do not know. Fall back to the display name, which
	// covers a provider that omits the code and the common spellings.
	switch {
	case e.BounceType == "":
		return bounceClassNone, false
	case strings.Contains(strings.ToLower(e.BounceType), "spam"),
		strings.Contains(strings.ToLower(e.BounceType), "virus"):
		return bounceClassComplaint, true
	case strings.EqualFold(e.BounceType, "HardBounce"),
		strings.EqualFold(e.BounceType, "BadEmailAddress"),
		strings.EqualFold(e.BounceType, "Blocked"):
		return bounceClassHard, true
	case strings.EqualFold(e.BounceType, "SoftBounce"),
		strings.EqualFold(e.BounceType, "Transient"),
		strings.EqualFold(e.BounceType, "DnsError"):
		return bounceClassSoft, true
	}

	return bounceClassNone, false
}
