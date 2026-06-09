package cli

import (
	"fmt"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/httpclient"
	"github.com/Jonathanm10/yt-cli/internal/output"
	"github.com/Jonathanm10/yt-cli/internal/youtrack"
)

func (a *App) resolveYouTrackService(common *commonFlags) (resolvedProfile, *youtrack.Service, error) {
	resolved, err := a.resolveProfile(common, true)
	if err != nil {
		return resolvedProfile{}, nil, err
	}
	service := youtrack.NewService(httpclient.New(resolved.BaseURL, resolved.Token, a.httpClient, common.Debug, a.stderr), resolved.BaseURL)
	return resolved, service, nil
}

func (a *App) writeServiceResult(service *youtrack.Service, common *commonFlags, resolved resolvedProfile, command string, normalized map[string]any, raw any) error {
	if common.Raw {
		return output.WriteJSON(a.stdout, service.RawEnvelope(raw, map[string]any{"command": command, "profile": resolved.Name}))
	}
	return output.WriteJSON(a.stdout, normalized)
}

func parseFields(values []string) ([]youtrack.FieldInput, error) {
	out := make([]youtrack.FieldInput, 0, len(values))
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, output.NewError(2, "validation_error", fmt.Sprintf("invalid custom field %q; expected NAME=VALUE", item), map[string]any{"field": item})
		}
		out = append(out, youtrack.FieldInput{Name: strings.TrimSpace(parts[0]), Value: strings.TrimSpace(parts[1])})
	}
	return out, nil
}
