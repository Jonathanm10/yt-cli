package youtrack

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeIssuePreservesPublicShape(t *testing.T) {
	raw := map[string]any{
		"id":          "2-1",
		"idReadable":  "YT-1",
		"summary":     "Fix boundary",
		"description": "Typed payloads",
		"project": map[string]any{
			"id":        "0-1",
			"shortName": "YT",
			"name":      "YouTrack",
		},
		"customFields": []any{
			map[string]any{
				"id":    "cf-1",
				"name":  "State",
				"$type": "StateIssueCustomField",
				"value": map[string]any{
					"$type": "StateBundleElement",
					"name":  "Open",
				},
			},
		},
		"attachments": []any{
			map[string]any{"id": "att-1", "name": "log.txt", "url": "/api/files/1"},
		},
		"updated": float64(123),
	}

	got := NormalizeIssue(raw, "https://example.youtrack.cloud")
	want := map[string]any{
		"schemaVersion": "v1",
		"issue": map[string]any{
			"id":          "2-1",
			"idReadable":  "YT-1",
			"summary":     "Fix boundary",
			"description": "Typed payloads",
			"project": map[string]any{
				"id":        "0-1",
				"shortName": "YT",
				"name":      "YouTrack",
			},
			"customFields": []map[string]any{
				{
					"id":   "cf-1",
					"name": "State",
					"type": "StateIssueCustomField",
					"value": map[string]any{
						"type": "StateBundleElement",
						"name": "Open",
					},
				},
			},
			"attachments": []map[string]any{
				{"id": "att-1", "name": "log.txt", "url": "/api/files/1"},
			},
			"links":   map[string]any{"web": "https://example.youtrack.cloud/issue/YT-1"},
			"updated": float64(123),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized issue mismatch:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestIssueCustomFieldRequestMarshalsYouTrackTypeKey(t *testing.T) {
	body := issueUpdateBody{
		CustomFields: []issueCustomFieldRequest{{
			Name:  "State",
			Type:  "StateIssueCustomField",
			Value: nameValue{Name: "Open"},
		}},
	}
	got, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"customFields":[{"name":"State","$type":"StateIssueCustomField","value":{"name":"Open"}}]}`
	if string(got) != want {
		t.Fatalf("request body mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}
