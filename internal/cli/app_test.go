package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestIssueCreateSerializesTypedCustomFields(t *testing.T) {
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
	if code := app.Run(context.Background(), []string{
		"issue", "create",
		"--profile", "sandbox",
		"--project", "SP",
		"--summary", "Typed create",
		"--field", "Type=User Story",
		"--field", "State=Backlog",
		"--field", "Acceptance Criteria=Ship it",
	}); code != 0 {
		t.Fatalf("create failed: code=%d stdout=%s", code, stdout.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.createRequests != 1 {
		t.Fatalf("expected one create request, got %d", fake.createRequests)
	}
	customFields, _ := fake.lastCreateBody["customFields"].([]any)
	if len(customFields) != 3 {
		t.Fatalf("expected three custom fields, got %#v", fake.lastCreateBody["customFields"])
	}

	typeField := customFields[0].(map[string]any)
	if got := fmt.Sprint(typeField["$type"]); got != "SingleEnumIssueCustomField" {
		t.Fatalf("expected typed enum field, got %q", got)
	}
	if got := fmt.Sprint(typeField["value"].(map[string]any)["name"]); got != "User Story" {
		t.Fatalf("unexpected enum value: %#v", typeField["value"])
	}

	stateField := customFields[1].(map[string]any)
	if got := fmt.Sprint(stateField["$type"]); got != "StateIssueCustomField" {
		t.Fatalf("expected typed state field, got %q", got)
	}
	if got := fmt.Sprint(stateField["value"].(map[string]any)["name"]); got != "Backlog" {
		t.Fatalf("unexpected state value: %#v", stateField["value"])
	}

	textField := customFields[2].(map[string]any)
	if got := fmt.Sprint(textField["$type"]); got != "TextIssueCustomField" {
		t.Fatalf("expected typed text field, got %q", got)
	}
	if got := fmt.Sprint(textField["value"].(map[string]any)["text"]); got != "Ship it" {
		t.Fatalf("unexpected text value: %#v", textField["value"])
	}
}

func TestIssueCreateSerializesTypedMultiEnumCustomField(t *testing.T) {
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
	if code := app.Run(context.Background(), []string{
		"issue", "create",
		"--profile", "sandbox",
		"--project", "SP",
		"--summary", "Multi enum create",
		"--field", "Platform=Android",
		"--field", "Platform=iOS",
	}); code != 0 {
		t.Fatalf("create failed: code=%d stdout=%s", code, stdout.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	customFields, _ := fake.lastCreateBody["customFields"].([]any)
	if len(customFields) != 1 {
		t.Fatalf("expected one grouped custom field, got %#v", fake.lastCreateBody["customFields"])
	}
	platformField := customFields[0].(map[string]any)
	if got := fmt.Sprint(platformField["$type"]); got != "MultiEnumIssueCustomField" {
		t.Fatalf("expected typed multi enum field, got %q", got)
	}
	values, _ := platformField["value"].([]any)
	if len(values) != 2 {
		t.Fatalf("expected two multi enum values, got %#v", platformField["value"])
	}
	if got := fmt.Sprint(values[0].(map[string]any)["name"]); got != "Android" {
		t.Fatalf("unexpected first platform value: %#v", values[0])
	}
	if got := fmt.Sprint(values[1].(map[string]any)["name"]); got != "iOS" {
		t.Fatalf("unexpected second platform value: %#v", values[1])
	}
}

func TestIssueUpdateSerializesTypedCustomFields(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	if code := app.Run(context.Background(), []string{"issue", "update", "SP-123", "--profile", "sandbox", "--field", "Type=User Story", "--field", "Acceptance Criteria=Done"}); code != 0 {
		t.Fatalf("update failed: %d", code)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.updateRequests != 1 {
		t.Fatalf("expected one update request, got %d", fake.updateRequests)
	}
	customFields, _ := fake.lastUpdateBody["customFields"].([]any)
	if len(customFields) != 2 {
		t.Fatalf("expected two custom fields, got %#v", fake.lastUpdateBody["customFields"])
	}
	if got := fmt.Sprint(customFields[0].(map[string]any)["$type"]); got != "SingleEnumIssueCustomField" {
		t.Fatalf("expected typed enum update, got %q", got)
	}
	if got := fmt.Sprint(customFields[1].(map[string]any)["$type"]); got != "TextIssueCustomField" {
		t.Fatalf("expected typed text update, got %q", got)
	}
}

func TestIssueUpdateSerializesTypedMultiEnumCustomField(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	if code := app.Run(context.Background(), []string{"issue", "update", "SP-123", "--profile", "sandbox", "--field", "Platform=Android"}); code != 0 {
		t.Fatalf("update failed: %d", code)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	customFields, _ := fake.lastUpdateBody["customFields"].([]any)
	if len(customFields) != 1 {
		t.Fatalf("expected one custom field, got %#v", fake.lastUpdateBody["customFields"])
	}
	platformField := customFields[0].(map[string]any)
	if got := fmt.Sprint(platformField["$type"]); got != "MultiEnumIssueCustomField" {
		t.Fatalf("expected typed multi enum update, got %q", got)
	}
	values, _ := platformField["value"].([]any)
	if len(values) != 1 || fmt.Sprint(values[0].(map[string]any)["name"]) != "Android" {
		t.Fatalf("unexpected platform values: %#v", platformField["value"])
	}
}

func TestIssueCreateFailsBeforeMutationForUnsupportedField(t *testing.T) {
	fake := newFakeYouTrack()
	fake.projectCustomFields["0-0"] = append(fake.projectCustomFields["0-0"], map[string]any{
		"id": "pcf-5", "$type": "UserProjectCustomField", "isPublic": true, "field": map[string]any{"name": "Owner"},
	})
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app.stdout = stdout
	app.stderr = stderr
	code := app.Run(context.Background(), []string{"issue", "create", "--profile", "sandbox", "--project", "SP", "--summary", "No mutation", "--field", "Owner=agent", "--json-errors"})
	if code != 2 {
		t.Fatalf("expected validation failure, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"unsupported_field_type"`) {
		t.Fatalf("unexpected JSON error output: %s", stdout.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.createRequests != 0 {
		t.Fatalf("expected zero create mutation requests, got %d", fake.createRequests)
	}
}

func TestIssueUpdateFailsBeforeMutationForInvisibleFieldMetadata(t *testing.T) {
	fake := newFakeYouTrack()
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	app := NewApp(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("valid-token\n"), ConfigDir: t.TempDir(), HTTPClient: server.Client()})
	app.openBrowser = func(string) error { return nil }
	if code := app.Run(context.Background(), []string{"auth", "login", "--profile", "sandbox", "--base-url", server.URL, "--token-stdin"}); code != 0 {
		t.Fatalf("login failed: %d", code)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app.stdout = stdout
	app.stderr = stderr
	code := app.Run(context.Background(), []string{"issue", "update", "SP-123", "--profile", "sandbox", "--field", "Hidden Field=secret", "--json-errors"})
	if code != 2 {
		t.Fatalf("expected metadata visibility failure, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"metadata_visibility_error"`) {
		t.Fatalf("unexpected JSON error output: %s", stdout.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.updateRequests != 0 {
		t.Fatalf("expected zero update mutation requests, got %d", fake.updateRequests)
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

func TestRootHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := NewApp(Options{Stdout: stdout, Stderr: stderr, ConfigDir: t.TempDir()})

	if code := app.Run(context.Background(), []string{"--help"}); code != 0 {
		t.Fatalf("help failed: code=%d stderr=%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "yt-cli - YouTrack CLI for terminal and automation") {
		t.Fatalf("missing root help header: %s", out)
	}
	if !strings.Contains(out, "Use \"yt-cli <command> --help\" for command-specific help.") {
		t.Fatalf("missing root help guidance: %s", out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %s", stderr.String())
	}
}

func TestCommandHelp(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		snippets []string
	}{
		{
			name: "auth",
			args: []string{"auth", "--help"},
			snippets: []string{
				"yt-cli auth <login|status|logout> [flags]",
				"yt-cli auth <command> --help",
			},
		},
		{
			name: "profile",
			args: []string{"profile", "--help"},
			snippets: []string{
				"yt-cli profile <list|use> [flags]",
				"yt-cli profile use sandbox",
			},
		},
		{
			name: "project",
			args: []string{"project", "--help"},
			snippets: []string{
				"yt-cli project list [--query TEXT] [flags]",
				"yt-cli project list --query ops --profile sandbox",
			},
		},
		{
			name: "issue",
			args: []string{"issue", "--help"},
			snippets: []string{
				"yt-cli issue <view|search|create|update|comment|transition|assign|attach> [flags]",
				"yt-cli issue <command> --help",
			},
		},
		{
			name: "workitem",
			args: []string{"workitem", "--help"},
			snippets: []string{
				"yt-cli workitem view ISSUE_ID [flags]",
				`compatibility alias for "yt-cli issue view"`,
			},
		},
		{
			name: "issue create leaf",
			args: []string{"issue", "create", "--help"},
			snippets: []string{
				"yt-cli issue create --summary TEXT",
				"--field NAME=VALUE",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			app := NewApp(Options{Stdout: stdout, Stderr: stderr, ConfigDir: t.TempDir()})

			if code := app.Run(context.Background(), tc.args); code != 0 {
				t.Fatalf("help failed: code=%d stderr=%s", code, stderr.String())
			}

			out := stdout.String()
			for _, snippet := range tc.snippets {
				if !strings.Contains(out, snippet) {
					t.Fatalf("expected help output to contain %q, got %s", snippet, out)
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected empty stderr, got %s", stderr.String())
			}
		})
	}
}

func TestHelpCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := NewApp(Options{Stdout: stdout, Stderr: &bytes.Buffer{}, ConfigDir: t.TempDir()})

	if code := app.Run(context.Background(), []string{"help", "issue", "search"}); code != 0 {
		t.Fatalf("help command failed: %d", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "yt-cli issue search --query TEXT") {
		t.Fatalf("unexpected help output: %s", out)
	}
}

func TestNormalizeFlagArgsUsesFlagSetMetadata(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	customBool := fs.Bool("custom-bool", false, "custom bool")
	customString := fs.String("custom-string", "", "custom string")

	args := normalizeFlagArgs(fs, []string{"pos", "--custom-string", "value", "--custom-bool", "tail"})
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if !*customBool {
		t.Fatal("expected custom bool flag to be set")
	}
	if *customString != "value" {
		t.Fatalf("expected custom string value, got %q", *customString)
	}
	if got := strings.Join(fs.Args(), ","); got != "pos,tail" {
		t.Fatalf("expected positionals to keep order, got %q", got)
	}
}

func TestCommandHelpSkipsCommonFlagValuesBeforeTopic(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := NewApp(Options{Stdout: stdout, Stderr: &bytes.Buffer{}, ConfigDir: t.TempDir()})

	if code := app.Run(context.Background(), []string{"issue", "--fields", "id,idReadable", "create", "--help"}); code != 0 {
		t.Fatalf("help command failed: %d", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "yt-cli issue create --summary TEXT") {
		t.Fatalf("unexpected help output: %s", out)
	}
}
