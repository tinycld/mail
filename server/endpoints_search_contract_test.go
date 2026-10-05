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
// response field can still be silently dropped: mapResults copies a subset of
// it into api.SearchResultItem. The row is deliberately WIDER than the wire
// type — it also carries the display columns (subject, participants, dates,
// counts, attachments) the federated search palette (search_source.go) reads
// straight off the row, bypassing mapResults entirely, because the web
// client resolves its own display from live rows but the palette has no live
// store to fall back on. So the invariant here is one-directional: every
// api.SearchResultItem json tag must have a matching searchResultRow db tag
// (wire ⊆ row) — not the other way around — and every value that IS on the
// wire must survive the copy under its wire key.
func TestMapResults_CoversEveryAPIField(t *testing.T) {
	rowType := reflect.TypeOf(searchResultRow{})
	itemType := reflect.TypeOf(api.SearchResultItem{})

	dbTags := tagSet(t, rowType, "db")
	jsonTags := tagSet(t, itemType, "json")

	for name := range jsonTags {
		if _, ok := dbTags[name]; !ok {
			t.Errorf("api.SearchResultItem json tag %q has no matching searchResultRow db tag", name)
		}
	}

	// Sentinel round-trip: distinct values in every WIRE field must surface in
	// the marshaled item under the same tag name — a crossed-wires copy in
	// mapResults fails here even though the field sets match. Row-only fields
	// (subject, participants, mailbox_id, has_attachments) are deliberately
	// absent from api.SearchResultItem, so they are not asserted against the
	// wire here.
	row := searchResultRow{
		ThreadID:         "sentinel-thread",
		StateID:          "sentinel-state",
		SubjectHighlight: "sentinel-subject-hl",
		SnippetHighlight: "sentinel-snippet-hl",
		LatestDate:       "sentinel-date",
		MessageCount:     7,
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
	for name := range jsonTags {
		fieldIdx := dbTags[name]
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
