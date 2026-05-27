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
	"runtime"
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
	if a.maybeHandleRootHelp(args) {
		return 0
	}
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
		return a.runProfile(args[1:])
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
	if a.maybeHandleCommandHelp("auth", args) {
		return nil
	}
	if len(args) == 0 {
		return output.NewError(2, "usage_error", "usage: yt-cli auth <login|status|logout>", nil)
	}
	switch args[0] {
	case "login":
		fs, common := a.newFlagSet("auth login")
		tokenStdin := fs.Bool("token-stdin", false, "read token from stdin")
		defaultProject := fs.String("default-project", "", "default project")
		if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
			return err
		}
		profileName := a.resolveProfileName(common.Profile)
		baseURL := firstNonEmpty(common.BaseURL, a.env["YTCLI_BASE_URL"])
		if strings.TrimSpace(baseURL) == "" {
			return output.NewError(2, "validation_error", "base URL is required (use --base-url or YTCLI_BASE_URL)", nil)
		}
		if !*tokenStdin {
			a.printStderr("Opening your YouTrack account page so you can complete SSO and create a permanent token from the Authentication tab.")
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
		if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
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
		if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
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

func (a *App) runProfile(args []string) error {
	if a.maybeHandleCommandHelp("profile", args) {
		return nil
	}
	if len(args) == 0 {
		return output.NewError(2, "usage_error", "usage: yt-cli profile <list|use>", nil)
	}
	switch args[0] {
	case "list":
		fs, common := a.newFlagSet("profile list")
		if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
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
		if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
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
	if a.maybeHandleCommandHelp("project", args) {
		return nil
	}
	if len(args) == 0 || args[0] != "list" {
		return output.NewError(2, "usage_error", "usage: yt-cli project list [--query TEXT]", nil)
	}
	fs, common := a.newFlagSet("project list")
	queryText := fs.String("query", "", "filter projects")
	if err := fs.Parse(normalizeFlagArgs(fs, args[1:])); err != nil {
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

func (a *App) maybeHandleRootHelp(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if isHelpFlag(args[0]) {
		a.printStdout(helpText(nil))
		return true
	}
	if args[0] == "help" {
		topics := a.helpTopics(args[1:], nil)
		if len(topics) > 2 {
			topics = topics[:2]
		}
		a.printStdout(helpText(topics))
		return true
	}
	return false
}

func (a *App) maybeHandleCommandHelp(command string, args []string) bool {
	if !hasHelpRequest(args) {
		return false
	}
	path := []string{command}
	topics := a.helpTopics(args, path)
	if len(topics) > 0 && topics[0] == "help" {
		topics = topics[1:]
	}
	if len(topics) > 0 {
		path = append(path, topics[0])
	}
	a.printStdout(helpText(path))
	return true
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

func (a *App) helpFlagSet(path []string) *flag.FlagSet {
	name := commandFlagSetName(path)
	fs, _ := a.newFlagSet(name)
	switch name {
	case "auth login":
		fs.Bool("token-stdin", false, "read token from stdin")
		fs.String("default-project", "", "default project")
	case "auth logout":
		fs.Bool("delete-profile", false, "delete the profile and its stored token")
	case "project list":
		fs.String("query", "", "filter projects")
	case "issue search":
		fs.String("query", "", "YouTrack query")
		fs.Int("top", 25, "page size")
		fs.Int("skip", 0, "offset")
	case "issue create":
		fs.String("project", "", "project short name")
		fs.String("summary", "", "issue summary")
		fs.String("description", "", "issue description")
		var fields stringList
		var attachments stringList
		fs.Var(&fields, "field", "custom field as Name=Value")
		fs.Var(&attachments, "attach", "attachment path")
	case "issue update":
		fs.String("summary", "", "issue summary")
		fs.String("description", "", "issue description")
		var fields stringList
		fs.Var(&fields, "field", "custom field as Name=Value")
	case "issue comment":
		fs.String("text", "", "comment text")
	case "issue transition":
		fs.String("state", "", "target state")
	case "issue assign":
		fs.String("user", "", "assignee login")
	}
	return fs
}

func commandFlagSetName(path []string) string {
	if len(path) == 0 {
		return ""
	}
	switch path[0] {
	case "auth":
		if len(path) > 1 {
			switch path[1] {
			case "login", "status", "logout":
				return "auth " + path[1]
			}
		}
	case "profile":
		if len(path) > 1 {
			switch path[1] {
			case "list", "use":
				return "profile " + path[1]
			}
		}
	case "project":
		if len(path) > 1 && path[1] == "list" {
			return "project list"
		}
	case "issue":
		if len(path) > 1 {
			switch path[1] {
			case "view", "search", "create", "update", "comment", "transition", "assign", "attach":
				return "issue " + path[1]
			}
		}
	case "workitem":
		if len(path) > 1 && path[1] == "view" {
			return "workitem view"
		}
	}
	return path[0]
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

func hasHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == "help" || isHelpFlag(arg) {
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

type boolFlag interface {
	IsBoolFlag() bool
}

func normalizeFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && flagExpectsValue(fs, arg) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func (a *App) helpTopics(args []string, initialPath []string) []string {
	topics := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "help" || isHelpFlag(arg) {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			path := append(append([]string{}, initialPath...), topics...)
			if !strings.Contains(arg, "=") && i+1 < len(args) && flagExpectsValue(a.helpFlagSet(path), arg) {
				i++
			}
			continue
		}
		topics = append(topics, arg)
	}
	return topics
}

func flagExpectsValue(fs *flag.FlagSet, arg string) bool {
	name := flagName(arg)
	if name == "" {
		return false
	}
	registered := fs.Lookup(name)
	if registered == nil {
		return false
	}
	if value, ok := registered.Value.(boolFlag); ok && value.IsBoolFlag() {
		return false
	}
	return true
}

func flagName(arg string) string {
	name := strings.TrimLeft(arg, "-")
	if beforeValue, _, ok := strings.Cut(name, "="); ok {
		name = beforeValue
	}
	return strings.TrimSpace(name)
}

func isHelpFlag(arg string) bool {
	switch arg {
	case "-h", "-help", "--help":
		return true
	default:
		return false
	}
}

func (a *App) printStderr(message string) {
	_, _ = fmt.Fprintln(a.stderr, message)
}

func (a *App) printStdout(message string) {
	_, _ = fmt.Fprint(a.stdout, message)
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
