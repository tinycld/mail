package mail

import "testing"

// Both arms of the UNION must carry the exclusion. Applying it to only one
// lets the other arm resurrect exactly the rows the user asked to drop —
// the most likely way this feature ships subtly broken.
func TestBothUnionArmsCarryExclusions(t *testing.T) {
	thread := buildThreadFTSQuery("budget", "draft")
	message := buildMessageFTSQuery("budget", "", "draft")

	for name, got := range map[string]string{"thread": thread, "message": message} {
		if !contains(got, `NOT "draft"*`) {
			t.Errorf("%s arm missing exclusion: %q", name, got)
		}
		if !contains(got, `"budget"*`) {
			t.Errorf("%s arm missing include term: %q", name, got)
		}
	}
}

func TestExclusionsOmittedWhenEmpty(t *testing.T) {
	if got := buildThreadFTSQuery("budget", ""); contains(got, "NOT") {
		t.Errorf("empty exclusion should emit no NOT, got %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && stringIndex(haystack, needle) >= 0
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
