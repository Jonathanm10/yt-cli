package cli

import (
	"context"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/output"
)

func (a *App) runIssueView(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue view")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return output.NewError(2, "validation_error", "usage: yt-cli issue view ISSUE_ID", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	normalized, raw, err := service.ViewIssue(ctx, fs.Arg(0), common.Fields)
	if err != nil {
		return mapHTTPError(err)
	}
	return a.writeServiceResult(service, common, resolved, "issue view", normalized, raw)
}

func (a *App) runIssueSearch(ctx context.Context, args []string) error {
	fs, common := a.newFlagSet("issue search")
	queryText := fs.String("query", "", "YouTrack query")
	top := fs.Int("top", 25, "page size")
	skip := fs.Int("skip", 0, "offset")
	if err := fs.Parse(normalizeFlagArgs(fs, args)); err != nil {
		return err
	}
	if strings.TrimSpace(*queryText) == "" {
		return output.NewError(2, "validation_error", "--query is required", nil)
	}
	resolved, service, err := a.resolveYouTrackService(common)
	if err != nil {
		return err
	}
	items, rawItems, hasMore, err := service.SearchIssues(ctx, *queryText, *top, *skip, common.Fields)
	if err != nil {
		return mapHTTPError(err)
	}
	normalized := output.SuccessEnvelope(map[string]any{"items": items, "page": pageEnvelope(*top, *skip, hasMore)})
	raw := map[string]any{"items": rawItems, "page": pageEnvelope(*top, *skip, hasMore)}
	return a.writeServiceResult(service, common, resolved, "issue search", normalized, raw)
}

func pageEnvelope(top, skip int, hasMore bool) map[string]any {
	return map[string]any{"top": top, "skip": skip, "hasMore": hasMore}
}
