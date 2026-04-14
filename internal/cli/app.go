package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/config"
	"github.com/Jonathanm10/yt-cli/internal/httpclient"
	"github.com/Jonathanm10/yt-cli/internal/output"
	"github.com/Jonathanm10/yt-cli/internal/youtrack"
)

type Options struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	Env        []string
	ConfigDir  string
	HTTPClient *http.Client
}

type App struct {
	stdout      io.Writer
	stderr      io.Writer
	stdin       io.Reader
	env         map[string]string
	store       *config.Store
	httpClient  *http.Client
	openBrowser func(string) error
}

type resolvedProfile struct {
	Name           string
	BaseURL        string
	DefaultProject string
	Token          string
	TokenSource    string
}

type commonFlags struct {
	Profile    string
	BaseURL    string
	JSONErrors bool
	Debug      bool
	Raw        bool
	Fields     string
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func NewApp(opts Options) *App {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	store, err := config.NewStore(opts.ConfigDir)
	if err != nil {
		panic(err)
	}
	return &App{
		stdout:      stdout,
		stderr:      stderr,
		stdin:       stdin,
		env:         makeEnvMap(opts.Env),
		store:       store,
		httpClient:  opts.HTTPClient,
		openBrowser: openBrowser,
	}
}

func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.fail(output.NewError(2, "usage_error", "usage: yt-cli <auth|profile|project|issue|workitem> ...", nil), false)
	}
	jsonErrors := contains(args, "--json-errors")
	err := a.run(ctx, args)
	if err != nil {
		return a.fail(err, jsonErrors)
	}
	return 0
}

func (a *App) run(ctx context.Context, args []string) error {
	switch args[0] {
	case "auth":
		return a.runAuth(ctx, args[1:])
	case "profile":
		return a.runProfile(ctx, args[1:])
	case "project":
		return a.runProject(ctx, args[1:])
	case "issue":
		return a.runIssue(ctx, args[1:])
	case "workitem":
		return a.runWorkItem(ctx, args[1:])
	default:
		return output.NewError(2, "usage_error", fmt.Sprintf("unknown command %q", args[0]), nil)
	}
}

func (a *App) runAuth(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return output.NewError(2, "usage_error", "usage: yt-cli auth <login|status|logout>", nil)
	}
	switch args[0] {
	case "login":
		fs, common := a.newFlagSet("auth login")
		tokenStdin := fs.Bool("token-stdin", false, "read token from stdin")
		defaultProject := fs.String("default-project", "", "default project")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		profileName := a.resolveProfileName(common.Profile)
		baseURL := firstNonEmpty(common.BaseURL, a.env["YTCLI_BASE_URL"])
		if strings.TrimSpace(baseURL) == "" {
			return output.NewError(2, "validation_error", "base URL is required (use --base-url or YTCLI_BASE_URL)", nil)
		}
		if !*tokenStdin {
			a.printStderr("Opening your hosted YouTrack account page so you can complete SSO and create a permanent token from the Authentication tab.")
			a.printStderr("After creating the token in the browser, paste it into the CLI.")
			if err := a.openBrowser(securityURL(baseURL)); err != nil {
				a.printStderr("Warning: could not open browser automatically: " + err.Error())
			}
		}
		token, err := a.readToken(*tokenStdin)
		if err != nil {
			return err
		}
		client := httpclient.New(baseURL, token, a.httpClient, common.Debug, a.stderr)
		service := youtrack.NewService(client, baseURL)
		if _, err := service.ValidateToken(ctx); err != nil {
			return mapHTTPError(err)
		}
		if err := a.store.UpsertProfile(config.Profile{Name: profileName, BaseURL: baseURL, DefaultProject: *defaultProject}); err != nil {
			return err
		}
		if err := a.store.SetActiveProfile(profileName); err != nil {
			return err
		}
		if err := a.store.SaveToken(profileName, token); err != nil {
			return err
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{
			"auth": map[string]any{
				"profile":        profileName,
				"baseUrl":        baseURL,
				"defaultProject": strings.TrimSpace(*defaultProject),
				"authenticated":  true,
				"tokenSource":    "stored",
			},
		}))
	case "status":
		fs, common := a.newFlagSet("auth status")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		me, err := service.ValidateToken(ctx)
		if err != nil {
			return mapHTTPError(err)
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{
			"auth": map[string]any{
				"profile":        resolved.Name,
				"baseUrl":        resolved.BaseURL,
				"defaultProject": resolved.DefaultProject,
				"authenticated":  true,
				"tokenSource":    resolved.TokenSource,
				"user":           me,
			},
		}))
	case "logout":
		fs, common := a.newFlagSet("auth logout")
		deleteProfile := fs.Bool("delete-profile", false, "delete the profile and its stored token")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		profileName := a.resolveProfileName(common.Profile)
		if *deleteProfile {
			if err := a.store.DeleteProfile(profileName); err != nil {
				return err
			}
		} else {
			if err := a.store.DeleteToken(profileName); err != nil {
				return err
			}
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{
			"auth": map[string]any{"profile": profileName, "loggedOut": true, "deletedProfile": *deleteProfile},
		}))
	default:
		return output.NewError(2, "usage_error", "usage: yt-cli auth <login|status|logout>", nil)
	}
}

func (a *App) runProfile(ctx context.Context, args []string) error {
	_ = ctx
	if len(args) == 0 {
		return output.NewError(2, "usage_error", "usage: yt-cli profile <list|use>", nil)
	}
	switch args[0] {
	case "list":
		fs, common := a.newFlagSet("profile list")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		profiles, active, err := a.store.ListProfiles()
		if err != nil {
			return err
		}
		items := make([]map[string]any, 0, len(profiles))
		for _, profile := range profiles {
			items = append(items, map[string]any{
				"name":           profile.Name,
				"baseUrl":        profile.BaseURL,
				"defaultProject": profile.DefaultProject,
				"active":         profile.Name == active,
				"authenticated":  a.store.HasToken(profile.Name),
			})
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{"items": items, "activeProfile": active, "raw": common.Raw}))
	case "use":
		fs, _ := a.newFlagSet("profile use")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return output.NewError(2, "validation_error", "usage: yt-cli profile use NAME", nil)
		}
		if err := a.store.SetActiveProfile(fs.Arg(0)); err != nil {
			return output.NewError(2, "validation_error", err.Error(), nil)
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{"profile": map[string]any{"name": fs.Arg(0), "active": true}}))
	default:
		return output.NewError(2, "usage_error", "usage: yt-cli profile <list|use>", nil)
	}
}

func (a *App) runProject(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return output.NewError(2, "usage_error", "usage: yt-cli project list [--query TEXT]", nil)
	}
	fs, common := a.newFlagSet("project list")
	queryText := fs.String("query", "", "filter projects")
	if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
		return err
	}
	resolved, err := a.resolveProfile(common, true)
	if err != nil {
		return err
	}
	service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
	projects, rawProjects, err := service.ListProjects(ctx, *queryText)
	if err != nil {
		return mapHTTPError(err)
	}
	if common.Raw {
		return output.WriteJSON(a.stdout, service.RawEnvelope(rawProjects, map[string]any{"command": "project list", "profile": resolved.Name}))
	}
	return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{"items": projects}))
}

func (a *App) runIssue(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return output.NewError(2, "usage_error", "usage: yt-cli issue <view|search|create|update|comment|transition|assign|attach>", nil)
	}
	switch args[0] {
	case "view":
		fs, common := a.newFlagSet("issue view")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return output.NewError(2, "validation_error", "usage: yt-cli issue view ISSUE_ID", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.ViewIssue(ctx, fs.Arg(0), common.Fields)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue view", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "search":
		fs, common := a.newFlagSet("issue search")
		queryText := fs.String("query", "", "YouTrack query")
		top := fs.Int("top", 25, "page size")
		skip := fs.Int("skip", 0, "offset")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if strings.TrimSpace(*queryText) == "" {
			return output.NewError(2, "validation_error", "--query is required", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		items, rawItems, hasMore, err := service.SearchIssues(ctx, *queryText, *top, *skip, common.Fields)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(map[string]any{"items": rawItems, "page": map[string]any{"top": *top, "skip": *skip, "hasMore": hasMore}}, map[string]any{"command": "issue search", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, output.SuccessEnvelope(map[string]any{"items": items, "page": map[string]any{"top": *top, "skip": *skip, "hasMore": hasMore}}))
	case "create":
		fs, common := a.newFlagSet("issue create")
		project := fs.String("project", "", "project short name")
		summary := fs.String("summary", "", "issue summary")
		description := fs.String("description", "", "issue description")
		var fields stringList
		var attachments stringList
		fs.Var(&fields, "field", "custom field as Name=Value")
		fs.Var(&attachments, "attach", "attachment path")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		chosenProject := firstNonEmpty(*project, resolved.DefaultProject)
		if strings.TrimSpace(chosenProject) == "" {
			return output.NewError(2, "validation_error", "project is required (use --project or set a default project)", nil)
		}
		if strings.TrimSpace(*summary) == "" {
			return output.NewError(2, "validation_error", "--summary is required", nil)
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, rawIssue, err := service.CreateIssue(ctx, youtrack.CreateInput{Project: chosenProject, Summary: *summary, Description: *description, Fields: parseFields(fields)})
		if err != nil {
			return mapHTTPError(err)
		}
		rawPayload := map[string]any{"issue": rawIssue}
		if len(attachments) > 0 {
			issueID := issueIDFromEnvelope(normalized)
			attachmentEnvelope, rawAttachments, attachErr := service.AttachFiles(ctx, issueID, attachments)
			if attachErr != nil {
				return output.NewError(5, "attach_partial_failure", "issue created but attachment upload failed", map[string]any{"createdIssue": normalized, "cause": attachErr.Error()})
			}
			issue := normalized["issue"].(map[string]any)
			issue["attachments"] = attachmentEnvelope["attachments"]
			rawPayload["attachments"] = rawAttachments
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(rawPayload, map[string]any{"command": "issue create", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "update":
		fs, common := a.newFlagSet("issue update")
		summary := fs.String("summary", "", "issue summary")
		description := fs.String("description", "", "issue description")
		var fields stringList
		fs.Var(&fields, "field", "custom field as Name=Value")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return output.NewError(2, "validation_error", "usage: yt-cli issue update ISSUE_ID [flags]", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.UpdateIssue(ctx, youtrack.UpdateInput{ID: fs.Arg(0), Summary: *summary, Description: *description, Fields: parseFields(fields)})
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue update", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "comment":
		fs, common := a.newFlagSet("issue comment")
		text := fs.String("text", "", "comment text")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*text) == "" {
			return output.NewError(2, "validation_error", "usage: yt-cli issue comment ISSUE_ID --text '...'", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.AddComment(ctx, fs.Arg(0), *text)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue comment", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "transition":
		fs, common := a.newFlagSet("issue transition")
		state := fs.String("state", "", "target state")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*state) == "" {
			return output.NewError(2, "validation_error", "usage: yt-cli issue transition ISSUE_ID --state '...'", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.TransitionIssue(ctx, fs.Arg(0), *state)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue transition", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "assign":
		fs, common := a.newFlagSet("issue assign")
		user := fs.String("user", "", "assignee login")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() != 1 || strings.TrimSpace(*user) == "" {
			return output.NewError(2, "validation_error", "usage: yt-cli issue assign ISSUE_ID --user login", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.AssignIssue(ctx, fs.Arg(0), *user)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue assign", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	case "attach":
		fs, common := a.newFlagSet("issue attach")
		if err := fs.Parse(normalizeFlagArgs(args[1:])); err != nil {
			return err
		}
		if fs.NArg() < 2 {
			return output.NewError(2, "validation_error", "usage: yt-cli issue attach ISSUE_ID file1 [file2 ...]", nil)
		}
		resolved, err := a.resolveProfile(common, true)
		if err != nil {
			return err
		}
		issueID := fs.Arg(0)
		files := fs.Args()[1:]
		service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
		normalized, raw, err := service.AttachFiles(ctx, issueID, files)
		if err != nil {
			return mapHTTPError(err)
		}
		if common.Raw {
			return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": "issue attach", "profile": resolved.Name}))
		}
		return output.WriteJSON(a.stdout, normalized)
	default:
		return output.NewError(2, "usage_error", "usage: yt-cli issue <view|search|create|update|comment|transition|assign|attach>", nil)
	}
}

func (a *App) runWorkItem(ctx context.Context, args []string) error {
	if len(args) >= 2 && args[0] == "view" {
		return a.runIssue(ctx, append([]string{"view"}, args[1:]...))
	}
	return output.NewError(2, "usage_error", "usage: yt-cli workitem view ISSUE_ID", nil)
}

func (a *App) newFlagSet(name string) (*flag.FlagSet, *commonFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	common := &commonFlags{}
	fs.StringVar(&common.Profile, "profile", "", "profile name")
	fs.StringVar(&common.BaseURL, "base-url", "", "YouTrack base URL")
	fs.BoolVar(&common.JSONErrors, "json-errors", false, "emit JSON errors")
	fs.BoolVar(&common.Debug, "debug", false, "emit debug logs to stderr")
	fs.BoolVar(&common.Raw, "raw", false, "emit raw API payload envelope")
	fs.StringVar(&common.Fields, "fields", "preset", "field preset")
	return fs, common
}

func (a *App) resolveProfile(common *commonFlags, requireToken bool) (resolvedProfile, error) {
	profilesFile, err := a.store.LoadProfiles()
	if err != nil {
		return resolvedProfile{}, err
	}
	profileName := a.resolveProfileName(common.Profile)
	profile := profilesFile.Profiles[profileName]
	baseURL := firstNonEmpty(common.BaseURL, a.env["YTCLI_BASE_URL"], profile.BaseURL)
	defaultProject := firstNonEmpty(a.env["YTCLI_DEFAULT_PROJECT"], profile.DefaultProject)
	storedToken, err := a.store.LoadToken(profileName)
	if err != nil {
		return resolvedProfile{}, err
	}
	token := strings.TrimSpace(a.env["YTCLI_TOKEN"])
	tokenSource := "env"
	if token == "" {
		token = storedToken
		tokenSource = "stored"
	} else if storedToken != "" && storedToken != token {
		a.printStderr("Warning: YTCLI_TOKEN overrides the stored token for the selected profile.")
	}
	if strings.TrimSpace(baseURL) == "" {
		return resolvedProfile{}, output.NewError(2, "validation_error", "base URL is required (use --base-url, YTCLI_BASE_URL, or an authenticated profile)", nil)
	}
	if requireToken && strings.TrimSpace(token) == "" {
		return resolvedProfile{}, output.NewError(3, "auth_error", "no authentication token available; run yt-cli auth login or set YTCLI_TOKEN", nil)
	}
	return resolvedProfile{Name: profileName, BaseURL: baseURL, DefaultProject: defaultProject, Token: token, TokenSource: tokenSource}, nil
}

func (a *App) resolveProfileName(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if envName := strings.TrimSpace(a.env["YTCLI_PROFILE"]); envName != "" {
		return envName
	}
	profilesFile, err := a.store.LoadProfiles()
	if err == nil && strings.TrimSpace(profilesFile.ActiveProfile) != "" {
		return profilesFile.ActiveProfile
	}
	return "default"
}

func (a *App) readToken(fromStdin bool) (string, error) {
	if fromStdin {
		data, err := io.ReadAll(a.stdin)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", output.NewError(2, "validation_error", "token is empty", nil)
		}
		return token, nil
	}
	a.printStderr("Paste your permanent token:")
	if file, ok := a.stdin.(*os.File); ok && isCharDevice(file) {
		if err := setTerminalEcho(file, false); err != nil {
			return "", output.NewError(5, "terminal_security_error", "unable to disable terminal echo for token entry; use --token-stdin instead", map[string]any{"cause": err.Error()})
		}
		defer func() {
			_ = setTerminalEcho(file, true)
			a.printStderr("")
		}()
	}
	reader := bufio.NewReader(a.stdin)
	token, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", output.NewError(2, "validation_error", "token is empty", nil)
	}
	return token, nil
}

func (a *App) fail(err error, jsonErrors bool) int {
	cliErr := normalizeError(err)
	if jsonErrors {
		_ = output.WriteJSON(a.stdout, output.ErrorEnvelope(cliErr))
	} else {
		output.WriteTextError(a.stderr, cliErr)
	}
	if cliErr.ExitCode == 0 {
		return 1
	}
	return cliErr.ExitCode
}

func normalizeError(err error) *output.CLIError {
	var cliErr *output.CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return output.NewError(5, "internal_error", err.Error(), nil)
}

func mapHTTPError(err error) error {
	var responseErr *httpclient.ResponseError
	if errors.As(err, &responseErr) {
		details := map[string]any{"status": responseErr.StatusCode}
		if responseErr.Payload != nil {
			details["response"] = responseErr.Payload
		}
		switch {
		case responseErr.StatusCode == 401 || responseErr.StatusCode == 403:
			return output.NewError(3, "auth_error", "authentication failed; re-run yt-cli auth login or refresh YTCLI_TOKEN", details)
		case responseErr.StatusCode == 404:
			return output.NewError(4, "not_found", "resource not found", details)
		case responseErr.StatusCode == 400 || responseErr.StatusCode == 422:
			return output.NewError(2, "validation_error", "request was rejected by YouTrack", details)
		default:
			return output.NewError(5, "api_error", "YouTrack request failed", details)
		}
	}
	return err
}

func parseFields(values []string) []youtrack.FieldInput {
	out := make([]youtrack.FieldInput, 0, len(values))
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, youtrack.FieldInput{Name: strings.TrimSpace(parts[0]), Value: strings.TrimSpace(parts[1])})
	}
	return out
}

func issueIDFromEnvelope(payload map[string]any) string {
	issue, _ := payload["issue"].(map[string]any)
	if idReadable := strings.TrimSpace(fmt.Sprint(issue["idReadable"])); idReadable != "" && idReadable != "<nil>" {
		return idReadable
	}
	return strings.TrimSpace(fmt.Sprint(issue["id"]))
}

func makeEnvMap(values []string) map[string]string {
	if len(values) == 0 {
		values = os.Environ()
	}
	env := map[string]string{}
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeFlagArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && flagExpectsValue(arg) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func flagExpectsValue(arg string) bool {
	switch arg {
	case "--json-errors", "--debug", "--raw", "--token-stdin":
		return false
	default:
		return true
	}
}

func (a *App) printStderr(message string) {
	_, _ = fmt.Fprintln(a.stderr, message)
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func securityURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return baseURL
	}
	return baseURL + "/hub/users/me?tab=auth"
}

func openBrowser(target string) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func SortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func FixturePath(parts ...string) string {
	return filepath.Join(parts...)
}
