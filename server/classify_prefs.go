package mail

import (
	"encoding/json"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/classifier"
)

const (
	classifierApp     = "mail"
	modeOff           = "off"
	modePreview       = "preview"
	modeLabels        = "labels"
	modeLabelsArchive = "labels_archive"

	prefMode                = "classifier.mode"
	prefTaxonomy            = "classifier.taxonomy"
	settingArchiveThreshold = "classifier.archive_threshold"
	defaultArchiveThreshold = 0.9
	minArchiveThreshold     = 0.5
)

// jsonString reads a json-typed value column that holds a JSON string.
func jsonString(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func userPref(app core.App, userID, key string) string {
	rec, err := app.FindFirstRecordByFilter("user_preferences", "user = {:u} && app = {:a} && key = {:k}",
		map[string]any{"u": userID, "a": classifierApp, "k": key})
	if err != nil {
		return ""
	}
	return jsonString(rec.Get("value"))
}

func userClassifierPrefs(app core.App, userID string) (mode, taxonomy string) {
	mode = userPref(app, userID, prefMode)
	switch mode {
	case modePreview, modeLabels, modeLabelsArchive:
	default:
		mode = modeOff
	}
	taxonomy = userPref(app, userID, prefTaxonomy)
	if taxonomy != "personal" {
		taxonomy = "org"
	}
	return mode, taxonomy
}

func archiveThreshold(app core.App) float64 {
	rec, err := app.FindFirstRecordByFilter("settings", "app = {:a} && key = {:k}",
		map[string]any{"a": classifierApp, "k": settingArchiveThreshold})
	if err != nil {
		return defaultArchiveThreshold
	}
	v, err := strconv.ParseFloat(jsonString(rec.Get("value")), 64)
	// Any signed-in member can write this org row, so the floor (the UI's
	// minimum) is enforced here: a low value would archive at any confidence.
	if err != nil || v < minArchiveThreshold || v > 1 {
		return defaultArchiveThreshold
	}
	return v
}

// loadTaxonomy reads one scope's rows in order. userID "" is the org default.
func loadTaxonomy(app core.App, userID string) []classifier.TaxonomyLabel {
	rows, err := app.FindRecordsByFilter("classifier_labels", "app = {:a} && user = {:u}", "order", 12, 0,
		map[string]any{"a": classifierApp, "u": userID})
	if err != nil {
		return nil
	}
	out := make([]classifier.TaxonomyLabel, 0, len(rows))
	for _, r := range rows {
		label, err := app.FindRecordById("labels", r.GetString("label"))
		if err != nil {
			continue
		}
		out = append(out, classifier.TaxonomyLabel{
			ID: r.Id, LabelID: label.Id, Name: label.GetString("name"),
			AssignWhen: r.GetString("assign_when"), AutoAction: r.GetBool("auto_action"),
		})
	}
	return out
}
