package cli

import (
	"context"

	"github.com/Jonathanm10/yt-cli/internal/output"
)

const issueUsage = "usage: yt-cli issue <view|search|create|update|comment|transition|assign|attach>"

func (a *App) runIssue(ctx context.Context, args []string) error {
	if a.maybeHandleCommandHelp("issue", args) {
		return nil
	}
	if len(args) == 0 {
		return output.NewError(2, "usage_error", issueUsage, nil)
	}

	switch args[0] {
	case "view":
		return a.runIssueView(ctx, args[1:])
	case "search":
		return a.runIssueSearch(ctx, args[1:])
	case "create":
		return a.runIssueCreate(ctx, args[1:])
	case "update":
		return a.runIssueUpdate(ctx, args[1:])
	case "comment":
		return a.runIssueComment(ctx, args[1:])
	case "transition":
		return a.runIssueTransition(ctx, args[1:])
	case "assign":
		return a.runIssueAssign(ctx, args[1:])
	case "attach":
		return a.runIssueAttach(ctx, args[1:])
	default:
		return output.NewError(2, "usage_error", issueUsage, nil)
	}
}

func (a *App) runWorkItem(ctx context.Context, args []string) error {
	if a.maybeHandleCommandHelp("workitem", args) {
		return nil
	}
	if len(args) >= 2 && args[0] == "view" {
		return a.runIssue(ctx, append([]string{"view"}, args[1:]...))
	}
	return output.NewError(2, "usage_error", "usage: yt-cli workitem view ISSUE_ID", nil)
}
