package mail

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"tinycld.org/packages/mail/api"
)

func tagSet(t *testing.T, typ reflect.Type, tagKey string) map[string]int {
	t.Helper()
	set := map[string]int{}
	for i := 0; i < typ.NumField(); i++ {
		tag, ok := typ.Field(i).Tag.Lookup(tagKey)
		if !ok {
			t.Fatalf("%s.%s has no %q tag", typ.Name(), typ.Field(i).Name, tagKey)
		}
		name, _, _ := strings.Cut(tag, ",")
		set[name] = i
	}
	return set
}

// searchResultRow (db tags, the SQL scan target) is the one place a search
// response field can still be silently dropped: mapResults copies it field by
// field into api.SearchResultItem. Pin the two field sets together and prove
// every value survives the copy under its wire key.
func TestMapResults_CoversEveryAPIField(t *testing.T) {
	rowType := reflect.TypeOf(searchResultRow{})
	itemType := reflect.TypeOf(api.SearchResultItem{})

	dbTags := tagSet(t, rowType, "db")
	jsonTags := tagSet(t, itemType, "json")

	for name := range dbTags {
		if _, ok := jsonTags[name]; !ok {
			t.Errorf("searchResultRow db tag %q has no matching api.SearchResultItem json tag", name)
		}
	}
	for name := range jsonTags {
		if _, ok := dbTags[name]; !ok {
			t.Errorf("api.SearchResultItem json tag %q has no matching searchResultRow db tag", name)
		}
	}

	// Sentinel round-trip: distinct values in every row field must surface in
	// the marshaled item under the same tag name — a crossed-wires copy in
	// mapResults fails here even though the field sets match.
	row := searchResultRow{
		ThreadID:         "sentinel-thread",
		Subject:          "sentinel-subject",
		SubjectHighlight: "sentinel-subject-hl",
		SnippetHighlight: "sentinel-snippet-hl",
		LatestDate:       "sentinel-date",
		Participants:     "sentinel-participants",
		MessageCount:     41,
		MailboxID:        "sentinel-mailbox",
		HasAttachments:   true,
	}
	items := mapResults([]searchResultRow{row})
	if len(items) != 1 {
		t.Fatalf("mapResults returned %d items, want 1", len(items))
	}
	marshaled, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(marshaled, &wire); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}

	rowValue := reflect.ValueOf(row)
	for name, fieldIdx := range dbTags {
		want := fmt.Sprint(rowValue.Field(fieldIdx).Interface())
		got, ok := wire[name]
		if !ok {
			t.Errorf("wire key %q missing from marshaled item", name)
			continue
		}
		if fmt.Sprint(got) != want {
			t.Errorf("wire key %q = %v, want %v — mapResults crossed wires", name, got, want)
		}
	}
}
