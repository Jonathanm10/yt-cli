package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
	mu               sync.Mutex
	issues           map[string]*fakeIssue
	projects         []map[string]any
	failTypedActions bool
	attachmentFail   string
	lastFields       string
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
		projects:         []map[string]any{{"id": "0-0", "shortName": "SP", "name": "Sample Project", "archived": true}},
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
		customFields, _ := body["customFields"].([]any)
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
	shortName := body["project"].(map[string]any)["shortName"]
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

func (f *fakeYouTrack) handleCommands(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	decodeJSON(r, &body)
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
		replaced := false
		for i, current := range out {
			if fmt.Sprint(current["name"]) == name {
				current["value"] = raw["value"]
				current["$type"] = chooseType(name)
				out[i] = current
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, map[string]any{"id": "cf-" + strings.ToLower(name), "name": name, "$type": chooseType(name), "value": raw["value"]})
		}
	}
	return out
}

func chooseType(name string) string {
	switch name {
	case "State":
		return "StateIssueCustomField"
	case "Assignee":
		return "SingleUserIssueCustomField"
	default:
		return "SimpleIssueCustomField"
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

func TestAuthLoginAndStatus(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := NewApp(Options{Stdout: stdout, Stderr: stderr, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }

	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL}); code != 0 {
		t.Fatalf("login failed: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"authenticated":true`) {
		t.Fatalf("unexpected login output: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"auth", "status", "--profile", "sandbox"}); code != 0 {
		t.Fatalf("status failed: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"login":"agent"`) {
		t.Fatalf("unexpected status output: %s", stdout.String())
	}
}

func TestIssueViewAliasAndRawMode(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	configDir := t.TempDir()
	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: configDir, HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	stdout1 := &bytes.Buffer{}
	app.stdout = stdout1
	app.stderr = &bytes.Buffer{}
	if code := app.Run(context.Background(), []string{"issue", "view", "SP-123", "--profile", "sandbox"}); code != 0 {
		t.Fatalf("issue view failed: %d", code)
	}

	stdout2 := &bytes.Buffer{}
	app.stdout = stdout2
	if code := app.Run(context.Background(), []string{"workitem", "view", "SP-123", "--profile", "sandbox"}); code != 0 {
		t.Fatalf("workitem view failed: %d", code)
	}
	if stdout1.String() != stdout2.String() {
		t.Fatalf("alias mismatch:\nissue=%s\nworkitem=%s", stdout1.String(), stdout2.String())
	}

	rawOut := &bytes.Buffer{}
	app.stdout = rawOut
	if code := app.Run(context.Background(), []string{"issue", "view", "SP-123", "--profile", "sandbox", "--raw"}); code != 0 {
		t.Fatalf("issue raw failed: %d", code)
	}
	if !strings.Contains(rawOut.String(), `"raw"`) {
		t.Fatalf("expected raw envelope: %s", rawOut.String())
	}
}

func TestTransitionFallsBackToCommandsAndSearchEnvelope(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	stdout := &bytes.Buffer{}
	app.stdout = stdout
	if code := app.Run(context.Background(), []string{"issue", "transition", "SP-123", "--profile", "sandbox", "--state", "In Progress"}); code != 0 {
		t.Fatalf("transition failed: %d", code)
	}
	if !strings.Contains(stdout.String(), `In Progress`) {
		t.Fatalf("unexpected transition output: %s", stdout.String())
	}

	searchOut := &bytes.Buffer{}
	app.stdout = searchOut
	if code := app.Run(context.Background(), []string{"issue", "search", "--profile", "sandbox", "--query", "in:SP", "--top", "1", "--skip", "0"}); code != 0 {
		t.Fatalf("search failed: %d", code)
	}
	if !strings.Contains(searchOut.String(), `"page":{"hasMore":true,"skip":0,"top":1}`) {
		t.Fatalf("unexpected search output: %s", searchOut.String())
	}
}

func TestCreateAttachPartialFailureAndJSONError(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	dir := t.TempDir()
	okFile := filepath.Join(dir, "ok.txt")
	failFile := filepath.Join(dir, "fail.txt")
	if err := os.WriteFile(okFile, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failFile, []byte("fail"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app.stdout = stdout
	app.stderr = stderr
	code := app.Run(context.Background(), []string{"issue", "create", "--profile", "sandbox", "--project", "SP", "--summary", "Created", "--attach", okFile, "--attach", failFile, "--json-errors"})
	if code != 5 {
		t.Fatalf("expected partial failure exit code 5, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `attach_partial_failure`) || !strings.Contains(stdout.String(), `createdIssue`) {
		t.Fatalf("unexpected JSON error payload: %s", stdout.String())
	}
}

func TestEnvTokenAndProjectList(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	stdout := &bytes.Buffer{}
	app := NewApp(Options{Stdout: stdout, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader(""), ConfigDir: t.TempDir(), HTTPClient: server.Client(), Env: []string{"YTCLI_TOKEN=valid-token", "YTCLI_BASE_URL=" + server.URL}})
	if code := app.Run(context.Background(), []string{"project", "list"}); code != 0 {
		t.Fatalf("project list failed: %d stdout=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"shortName":"SP"`) {
		t.Fatalf("unexpected project list output: %s", stdout.String())
	}

	stdout.Reset()
	if code := app.Run(context.Background(), []string{"project", "list", "--raw"}); code != 0 {
		t.Fatalf("project list raw failed: %d stdout=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"archived":true`) {
		t.Fatalf("raw project payload did not preserve upstream fields: %s", stdout.String())
	}
}

func TestAttachmentUsesUploadFieldName(t *testing.T) {
	var captured []*multipart.FileHeader
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users/me" {
			io.WriteString(w, `{"id":"1-1","login":"agent"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			_ = r.ParseMultipartForm(10 << 20)
			captured = r.MultipartForm.File["upload"]
			io.WriteString(w, `[{"id":"a-1","name":"sample.txt","url":"/files/sample.txt"}]`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/issues/") {
			io.WriteString(w, `{"id":"2-38","idReadable":"SP-123","summary":"Existing issue","description":"Investigate","project":{"id":"0-0","shortName":"SP","name":"Sample Project"},"customFields":[]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}
	if code := app.Run(context.Background(), []string{"issue", "attach", "SP-123", file, "--profile", "sandbox"}); code != 0 {
		t.Fatalf("attach failed: %d", code)
	}
	if len(captured) != 1 || captured[0].Filename != "sample.txt" {
		t.Fatalf("upload field was not sent as expected: %#v", captured)
	}
}

func TestIssueViewUsesExplicitFieldsFlag(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}
	if code := app.Run(context.Background(), []string{"issue", "view", "SP-123", "--profile", "sandbox", "--fields", "id,idReadable"}); code != 0 {
		t.Fatalf("view failed: %d", code)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastFields != "id,idReadable" {
		t.Fatalf("expected explicit fields to be forwarded, got %q", fake.lastFields)
	}
}

func TestAuthLogoutDeleteProfile(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	configDir := t.TempDir()
	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: configDir, HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}
	if code := app.Run(context.Background(), []string{"auth", "logout", "--profile", "sandbox", "--delete-profile"}); code != 0 {
		t.Fatalf("logout failed: %d", code)
	}
	profiles, err := app.store.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profiles.Profiles["sandbox"]; ok {
		t.Fatal("expected sandbox profile to be deleted")
	}
}
