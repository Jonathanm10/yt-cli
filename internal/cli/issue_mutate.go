package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/output"
	"github.com/Jonathanm10/yt-cli/internal/youtrack"
)

func (a *App) runIssueCreate(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue create")
	project := fs.String("project", "", "project short name")
	summary := fs.String("summary", "", "issue summary")
	description := fs.String("description", "", "issue description")
	var fields stringList
	var attachments stringList
	fs.Var(&fields, "field", "custom field as Name=Value")
	fs.Var(&attachments, "attach", "attachment path")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	parsedFields, err := parseFields(fields)
	if err != nil {
		return err
	}
	resolved, service, err := a.resolveYouTrackService(common)
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
	normalized, rawIssue, err := service.CreateIssue(ctx, youtrack.CreateInput{Project: chosenProject, Summary: *summary, Description: *description, Fields: parsedFields})
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
	return a.writeServiceResult(service, common, resolved, "issue create", normalized, rawPayload)
}

func (a *App) runIssueUpdate(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue update")
	summary := fs.String("summary", "", "issue summary")
	description := fs.String("description", "", "issue description")
	var fields stringList
	fs.Var(&fields, "field", "custom field as Name=Value")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return output.NewError(2, "validation_error", "usage: yt-cli issue update ISSUE_ID [flags]", nil)
	}
	parsedFields, err := parseFields(fields)
	if err != nil {
		return err
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.UpdateIssue(ctx, youtrack.UpdateInput{ID: fs.Arg(0), Summary: *summary, Description: *description, Fields: parsedFields})
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue update", normalized, raw)
}

func (a *App) runIssueComment(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue comment")
	text := fs.String("text", "", "comment text")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*text) == "" {
		return output.NewError(2, "validation_error", "usage: yt-cli issue comment ISSUE_ID --text '...'", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.AddComment(ctx, fs.Arg(0), *text)
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue comment", normalized, raw)
}

func (a *App) runIssueTransition(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue transition")
	state := fs.String("state", "", "target state")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*state) == "" {
		return output.NewError(2, "validation_error", "usage: yt-cli issue transition ISSUE_ID --state '...'", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.TransitionIssue(ctx, fs.Arg(0), *state)
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue transition", normalized, raw)
}

func (a *App) runIssueAssign(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue assign")
	user := fs.String("user", "", "assignee login")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*user) == "" {
		return output.NewError(2, "validation_error", "usage: yt-cli issue assign ISSUE_ID --user login", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.AssignIssue(ctx, fs.Arg(0), *user)
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue assign", normalized, raw)
}

func (a *App) runIssueAttach(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue attach")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return output.NewError(2, "validation_error", "usage: yt-cli issue attach ISSUE_ID file1 [file2 ...]", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.AttachFiles(ctx, fs.Arg(0), fs.Args()[1:])
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue attach", normalized, raw)
}

func issueIDFromEnvelope(payload map[string]any) string {
	issue, _ := payload["issue"].(map[string]any)
	if idReadable := strings.TrimSpace(fmt.Sprint(issue["idReadable"])); idReadable != "" && idReadable != "<nil>" {
		return idReadable
	}
	return strings.TrimSpace(fmt.Sprint(issue["id"]))
}
