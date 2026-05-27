package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type fakeIssue struct {
	ID           string
	IDReadable   string
	Summary      string
	Description  string
	Project      map[string]any
	CustomFields []map[string]any
	Attachments  []map[string]any
	Comments     []map[string]any
}

type fakeYouTrack struct {
	mu                  sync.Mutex
	issues              map[string]*fakeIssue
	projects            []map[string]any
	projectCustomFields map[string][]map[string]any
	failTypedActions    bool
	attachmentFail      string
	lastFields          string
	createRequests      int
	updateRequests      int
	commandRequests     int
	lastCreateBody      map[string]any
	lastUpdateBody      map[string]any
}

func newFakeYouTrack() *fakeYouTrack {
	return &fakeYouTrack{
		issues: map[string]*fakeIssue{
			"SP-123": {
				ID: "2-38", IDReadable: "SP-123", Summary: "Existing issue", Description: "Investigate thing",
				Project: map[string]any{"id": "0-0", "shortName": "SP", "name": "Sample Project"},
				CustomFields: []map[string]any{
					{"id": "58-1", "name": "Priority", "$type": "SingleEnumIssueCustomField", "value": "Normal"},
					{"id": "58-2", "name": "State", "$type": "StateIssueCustomField", "value": map[string]any{"name": "Open"}},
				},
			},
		},
		projects: []map[string]any{{"id": "0-0", "shortName": "SP", "name": "Sample Project", "archived": true}},
		projectCustomFields: map[string][]map[string]any{
			"0-0": {
				{"id": "pcf-1", "$type": "EnumProjectCustomField", "isPublic": true, "field": map[string]any{"name": "Type"}},
				{"id": "pcf-2", "$type": "StateProjectCustomField", "isPublic": true, "field": map[string]any{"name": "State"}},
				{"id": "pcf-3", "$type": "TextProjectCustomField", "isPublic": true, "field": map[string]any{"name": "Acceptance Criteria"}},
				{"id": "pcf-4", "$type": "EnumProjectCustomField", "isPublic": true, "field": map[string]any{"name": "Priority"}},
				{"id": "pcf-5", "$type": "TextProjectCustomField", "isPublic": false, "field": map[string]any{"name": "Hidden Field"}},
			},
		},
		failTypedActions: true,
		attachmentFail:   "fail.txt",
	}
}

func (f *fakeYouTrack) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users/me":
			io.WriteString(w, `{"id":"1-1","login":"agent","name":"Agent User"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/projects":
			mustJSON(w, f.projects)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/admin/projects/") && strings.HasSuffix(r.URL.Path, "/customFields"):
			projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/admin/projects/"), "/customFields")
			fields, ok := f.projectCustomFields[projectID]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"missing"}`)
				return
			}
			mustJSON(w, fields)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues":
			f.mu.Lock()
			defer f.mu.Unlock()
			items := make([]any, 0, len(f.issues))
			for _, issue := range f.issues {
				items = append(items, f.issuePayload(issue))
			}
			mustJSON(w, items)
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues":
			f.handleCreateIssue(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/issues/"):
			f.handleIssueRoutes(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/commands":
			f.handleCommands(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"not found"}`)
		}
	})
}

func (f *fakeYouTrack) handleIssueRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/issues/")
	switch {
	case r.Method == http.MethodGet:
		f.mu.Lock()
		f.lastFields = r.URL.Query().Get("fields")
		f.mu.Unlock()
		issue := f.lookupIssue(path)
		if issue == nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"missing"}`)
			return
		}
		mustJSON(w, f.issuePayload(issue))
	case r.Method == http.MethodPost && !strings.Contains(path, "/"):
		issue := f.lookupIssue(path)
		if issue == nil {
			f.handleCreateIssue(w, r)
			return
		}
		var body map[string]any
		decodeJSON(r, &body)
		f.mu.Lock()
		f.updateRequests++
		f.lastUpdateBody = cloneMap(body)
		f.mu.Unlock()
		customFields, _ := body["customFields"].([]any)
		if rejectLooseCustomFields(customFields) {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"Bad Request","error_description":"$type is required"}`)
			return
		}
		if f.failTypedActions && len(customFields) > 0 {
			names := []string{}
			for _, item := range customFields {
				raw := item.(map[string]any)
				names = append(names, fmt.Sprint(raw["name"]))
			}
			if slicesContain(names, "State") || slicesContain(names, "Assignee") {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"error":"typed action rejected"}`)
				return
			}
		}
		if summary := strings.TrimSpace(fmt.Sprint(body["summary"])); summary != "" && summary != "<nil>" {
			issue.Summary = summary
		}
		if description, ok := body["description"].(string); ok {
			issue.Description = description
		}
		if len(customFields) > 0 {
			issue.CustomFields = mergeCustomFields(issue.CustomFields, customFields)
		}
		mustJSON(w, f.issuePayload(issue))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/comments"):
		issue := f.lookupIssue(strings.TrimSuffix(path, "/comments"))
		if issue == nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"missing"}`)
			return
		}
		var body map[string]any
		decodeJSON(r, &body)
		comment := map[string]any{"id": "c-1", "text": body["text"], "author": map[string]any{"login": "agent", "name": "Agent User"}}
		issue.Comments = append(issue.Comments, comment)
		mustJSON(w, comment)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/attachments"):
		issue := f.lookupIssue(strings.TrimSuffix(path, "/attachments"))
		if issue == nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":"missing"}`)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"bad multipart"}`)
			return
		}
		files := r.MultipartForm.File["upload"]
		results := []map[string]any{}
		for _, fh := range files {
			if fh.Filename == f.attachmentFail {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"error":"attachment failed"}`)
				return
			}
			attachment := map[string]any{"id": "a-" + fh.Filename, "name": fh.Filename, "url": "/files/" + fh.Filename}
			issue.Attachments = append(issue.Attachments, attachment)
			results = append(results, attachment)
		}
		mustJSON(w, results)
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"missing"}`)
	}
}

func (f *fakeYouTrack) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	decodeJSON(r, &body)
	f.mu.Lock()
	f.createRequests++
	f.lastCreateBody = cloneMap(body)
	f.mu.Unlock()
	projectBody := body["project"].(map[string]any)
	shortName := fmt.Sprint(projectBody["shortName"])
	if shortName == "" || shortName == "<nil>" {
		projectID := fmt.Sprint(projectBody["id"])
		shortName = f.projectShortName(projectID)
	}
	customFields, _ := body["customFields"].([]any)
	if rejectLooseCustomFields(customFields) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"Bad Request","error_description":"$type is required"}`)
		return
	}
	issue := &fakeIssue{
		ID: "2-99", IDReadable: "SP-999", Summary: fmt.Sprint(body["summary"]), Description: fmt.Sprint(body["description"]),
		Project:      map[string]any{"id": "0-0", "shortName": shortName, "name": "Sample Project"},
		CustomFields: []map[string]any{},
	}
	if customFields, ok := body["customFields"].([]any); ok {
		issue.CustomFields = mergeCustomFields(nil, customFields)
	}
	f.mu.Lock()
	f.issues[issue.IDReadable] = issue
	f.mu.Unlock()
	mustJSON(w, f.issuePayload(issue))
}

func (f *fakeYouTrack) projectShortName(projectID string) string {
	for _, project := range f.projects {
		if fmt.Sprint(project["id"]) == projectID {
			return fmt.Sprint(project["shortName"])
		}
	}
	return ""
}

func (f *fakeYouTrack) handleCommands(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	decodeJSON(r, &body)
	f.mu.Lock()
	f.commandRequests++
	f.mu.Unlock()
	issues := body["issues"].([]any)
	issueID := fmt.Sprint(issues[0].(map[string]any)["idReadable"])
	issue := f.lookupIssue(issueID)
	if issue == nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"missing"}`)
		return
	}
	query := fmt.Sprint(body["query"])
	switch {
	case strings.HasPrefix(query, "State "):
		issue.CustomFields = mergeCustomFields(issue.CustomFields, []any{map[string]any{"name": "State", "value": map[string]any{"name": strings.TrimPrefix(query, "State ")}}})
	case strings.HasPrefix(query, "Assignee "):
		issue.CustomFields = mergeCustomFields(issue.CustomFields, []any{map[string]any{"name": "Assignee", "value": map[string]any{"login": strings.TrimPrefix(query, "Assignee "), "name": strings.TrimPrefix(query, "Assignee ")}}})
	}
	mustJSON(w, f.issuePayload(issue))
}

func (f *fakeYouTrack) lookupIssue(key string) *fakeIssue {
	f.mu.Lock()
	defer f.mu.Unlock()
	if issue, ok := f.issues[key]; ok {
		return issue
	}
	for _, issue := range f.issues {
		if issue.ID == key {
			return issue
		}
	}
	return nil
}

func (f *fakeYouTrack) issuePayload(issue *fakeIssue) map[string]any {
	return map[string]any{
		"id":           issue.ID,
		"idReadable":   issue.IDReadable,
		"summary":      issue.Summary,
		"description":  issue.Description,
		"project":      issue.Project,
		"customFields": issue.CustomFields,
		"attachments":  issue.Attachments,
	}
}

func mergeCustomFields(existing []map[string]any, updates []any) []map[string]any {
	out := append([]map[string]any{}, existing...)
	for _, item := range updates {
		raw := item.(map[string]any)
		name := fmt.Sprint(raw["name"])
		fieldType := firstNonEmpty(fmt.Sprint(raw["$type"]), chooseType(name))
		replaced := false
		for i, current := range out {
			if fmt.Sprint(current["name"]) == name {
				current["value"] = raw["value"]
				current["$type"] = fieldType
				out[i] = current
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, map[string]any{"id": "cf-" + strings.ToLower(name), "name": name, "$type": fieldType, "value": raw["value"]})
		}
	}
	return out
}

func chooseType(name string) string {
	switch name {
	case "Type", "Priority":
		return "SingleEnumIssueCustomField"
	case "State":
		return "StateIssueCustomField"
	case "Assignee":
		return "SingleUserIssueCustomField"
	case "Acceptance Criteria":
		return "TextIssueCustomField"
	default:
		return "SimpleIssueCustomField"
	}
}

func rejectLooseCustomFields(customFields []any) bool {
	for _, item := range customFields {
		raw, ok := item.(map[string]any)
		if !ok {
			return true
		}
		if strings.TrimSpace(fmt.Sprint(raw["$type"])) == "" || raw["$type"] == "<nil>" {
			return true
		}
	}
	return false
}

func cloneMap(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	out := make(map[string]any, len(v))
	for k, value := range v {
		out[k] = cloneValue(value)
	}
	return out
}

func cloneSlice(v []any) []any {
	out := make([]any, 0, len(v))
	for _, value := range v {
		out = append(out, cloneValue(value))
	}
	return out
}

func cloneValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		return cloneMap(value)
	case []any:
		return cloneSlice(value)
	default:
		return value
	}
}

func decodeJSON(r *http.Request, v any) {
	_ = json.NewDecoder(r.Body).Decode(v)
}

func mustJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
