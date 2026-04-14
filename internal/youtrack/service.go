package youtrack

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/httpclient"
	"github.com/Jonathanm10/yt-cli/internal/output"
)

const issueFields = "id,idReadable,summary,description,project(id,shortName,name),customFields(id,name,$type,value(id,login,name,presentation,text)),attachments(id,name,url),updated"
const projectFields = "id,shortName,name"

type Service struct {
	client  *httpclient.Client
	baseURL string
}

type FieldInput struct {
	Name  string
	Value string
}

type CreateInput struct {
	Project     string
	Summary     string
	Description string
	Fields      []FieldInput
}

type UpdateInput struct {
	ID          string
	Summary     string
	Description string
	Fields      []FieldInput
}

func NewService(client *httpclient.Client, baseURL string) *Service {
	return &Service{client: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func (s *Service) ValidateToken(ctx context.Context) (map[string]any, error) {
	query := url.Values{"fields": []string{"id,login,name"}}
	payload, err := s.client.DoJSON(ctx, "GET", "/api/users/me", query, nil)
	if err != nil {
		return nil, err
	}
	return asMap(payload), nil
}

func (s *Service) ListProjects(ctx context.Context, queryText string) ([]map[string]any, []any, error) {
	query := url.Values{"fields": []string{projectFields}}
	if strings.TrimSpace(queryText) != "" {
		query.Set("query", queryText)
	}
	payload, err := s.client.DoJSON(ctx, "GET", "/api/admin/projects", query, nil)
	if err != nil {
		return nil, nil, err
	}
	items := asSlice(payload)
	projects := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw := asMap(item)
		projects = append(projects, map[string]any{
			"id":        raw["id"],
			"shortName": raw["shortName"],
			"name":      raw["name"],
		})
	}
	return projects, items, nil
}

func (s *Service) ViewIssue(ctx context.Context, issueID, fields string) (map[string]any, map[string]any, error) {
	payload, err := s.client.DoJSON(ctx, "GET", "/api/issues/"+url.PathEscape(issueID), url.Values{"fields": []string{resolveIssueFields(fields)}}, nil)
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	return NormalizeIssue(raw, s.baseURL), raw, nil
}

func (s *Service) SearchIssues(ctx context.Context, queryText string, top, skip int, fields string) ([]map[string]any, []any, bool, error) {
	query := url.Values{"query": []string{queryText}, "fields": []string{resolveIssueFields(fields)}, "$top": []string{fmt.Sprintf("%d", top)}, "$skip": []string{fmt.Sprintf("%d", skip)}}
	payload, err := s.client.DoJSON(ctx, "GET", "/api/issues", query, nil)
	if err != nil {
		return nil, nil, false, err
	}
	rawItems := asSlice(payload)
	normalized := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		normalized = append(normalized, NormalizeIssue(asMap(item), s.baseURL))
	}
	hasMore := len(rawItems) == top
	return normalized, rawItems, hasMore, nil
}

func (s *Service) CreateIssue(ctx context.Context, in CreateInput) (map[string]any, map[string]any, error) {
	body := map[string]any{
		"project": map[string]any{"shortName": in.Project},
		"summary": in.Summary,
	}
	if strings.TrimSpace(in.Description) != "" {
		body["description"] = in.Description
	}
	if len(in.Fields) > 0 {
		body["customFields"] = toFieldPayload(in.Fields)
	}
	payload, err := s.client.DoJSON(ctx, "POST", "/api/issues", url.Values{"fields": []string{issueFields}}, body)
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	return NormalizeIssue(raw, s.baseURL), raw, nil
}

func (s *Service) UpdateIssue(ctx context.Context, in UpdateInput) (map[string]any, map[string]any, error) {
	body := map[string]any{}
	if strings.TrimSpace(in.Summary) != "" {
		body["summary"] = in.Summary
	}
	if in.Description != "" {
		body["description"] = in.Description
	}
	if len(in.Fields) > 0 {
		body["customFields"] = toFieldPayload(in.Fields)
	}
	payload, err := s.client.DoJSON(ctx, "POST", "/api/issues/"+url.PathEscape(in.ID), url.Values{"fields": []string{issueFields}}, body)
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	return NormalizeIssue(raw, s.baseURL), raw, nil
}

func (s *Service) AddComment(ctx context.Context, issueID, text string) (map[string]any, map[string]any, error) {
	payload, err := s.client.DoJSON(ctx, "POST", "/api/issues/"+url.PathEscape(issueID)+"/comments", url.Values{"fields": []string{"id,text,author(login,name)"}}, map[string]any{"text": text})
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	normalized := map[string]any{
		"schemaVersion": output.SchemaVersion,
		"comment": map[string]any{
			"id":     raw["id"],
			"text":   raw["text"],
			"author": normalizeValue(raw["author"]),
		},
	}
	return normalized, raw, nil
}

func (s *Service) TransitionIssue(ctx context.Context, issueID, state string) (map[string]any, map[string]any, error) {
	if normalized, raw, err := s.tryTypedAction(ctx, issueID, "State", map[string]any{"name": state}); err == nil {
		return normalized, raw, nil
	} else if !isFallbackable(err) {
		return nil, nil, err
	}
	return s.applyCommand(ctx, issueID, "State "+state)
}

func (s *Service) AssignIssue(ctx context.Context, issueID, user string) (map[string]any, map[string]any, error) {
	if normalized, raw, err := s.tryTypedAction(ctx, issueID, "Assignee", map[string]any{"login": user}); err == nil {
		return normalized, raw, nil
	} else if !isFallbackable(err) {
		return nil, nil, err
	}
	return s.applyCommand(ctx, issueID, "Assignee "+user)
}

func (s *Service) AttachFiles(ctx context.Context, issueID string, filePaths []string) (map[string]any, []any, error) {
	payload, err := s.client.DoMultipart(ctx, "/api/issues/"+url.PathEscape(issueID)+"/attachments", url.Values{"fields": []string{"id,name,url"}}, "upload", filePaths)
	if err != nil {
		return nil, nil, err
	}
	rawItems := asSlice(payload)
	items := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		raw := asMap(item)
		items = append(items, map[string]any{
			"id":   raw["id"],
			"name": raw["name"],
			"url":  raw["url"],
		})
	}
	return map[string]any{"schemaVersion": output.SchemaVersion, "attachments": items}, rawItems, nil
}

func (s *Service) RawEnvelope(payload any, meta map[string]any) map[string]any {
	return map[string]any{
		"schemaVersion": output.SchemaVersion,
		"meta":          meta,
		"raw":           payload,
	}
}

func resolveIssueFields(fields string) string {
	if strings.TrimSpace(fields) == "" || strings.TrimSpace(fields) == "preset" {
		return issueFields
	}
	return strings.TrimSpace(fields)
}

func NormalizeIssue(raw map[string]any, baseURL string) map[string]any {
	issue := map[string]any{
		"id":           raw["id"],
		"idReadable":   raw["idReadable"],
		"summary":      raw["summary"],
		"description":  raw["description"],
		"project":      normalizeProject(raw["project"]),
		"customFields": normalizeCustomFields(raw["customFields"]),
		"attachments":  normalizeAttachments(raw["attachments"]),
		"links": map[string]any{
			"web": buildIssueURL(baseURL, raw["idReadable"]),
		},
	}
	if raw["updated"] != nil {
		issue["updated"] = raw["updated"]
	}
	return map[string]any{"schemaVersion": output.SchemaVersion, "issue": issue}
}

func normalizeProject(v any) map[string]any {
	raw := asMap(v)
	return map[string]any{"id": raw["id"], "shortName": raw["shortName"], "name": raw["name"]}
}

func normalizeCustomFields(v any) []map[string]any {
	items := asSlice(v)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw := asMap(item)
		out = append(out, map[string]any{
			"id":    raw["id"],
			"name":  raw["name"],
			"type":  firstNonEmpty(fmt.Sprint(raw["$type"]), fmt.Sprint(raw["type"])),
			"value": normalizeValue(raw["value"]),
		})
	}
	return out
}

func normalizeAttachments(v any) []map[string]any {
	items := asSlice(v)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw := asMap(item)
		out = append(out, map[string]any{"id": raw["id"], "name": raw["name"], "url": raw["url"]})
	}
	return out
}

func normalizeValue(v any) any {
	switch value := v.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, child := range value {
			if k == "$type" {
				out["type"] = child
				continue
			}
			out[k] = normalizeValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(value))
		for _, child := range value {
			out = append(out, normalizeValue(child))
		}
		return out
	default:
		return value
	}
}

func buildIssueURL(baseURL string, id any) string {
	idReadable := fmt.Sprint(id)
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(idReadable) == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	u.Path = path.Join(strings.TrimSuffix(u.Path, "/"), "issue", idReadable)
	return u.String()
}

func (s *Service) tryTypedAction(ctx context.Context, issueID, fieldName string, value any) (map[string]any, map[string]any, error) {
	body := map[string]any{"customFields": []map[string]any{{"name": fieldName, "value": value}}}
	payload, err := s.client.DoJSON(ctx, "POST", "/api/issues/"+url.PathEscape(issueID), url.Values{"fields": []string{issueFields}}, body)
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	return NormalizeIssue(raw, s.baseURL), raw, nil
}

func (s *Service) applyCommand(ctx context.Context, issueID, queryText string) (map[string]any, map[string]any, error) {
	body := map[string]any{
		"query":  queryText,
		"issues": []map[string]any{{"idReadable": issueID}},
	}
	payload, err := s.client.DoJSON(ctx, "POST", "/api/commands", url.Values{"fields": []string{issueFields}}, body)
	if err != nil {
		return nil, nil, err
	}
	raw := asMap(payload)
	return NormalizeIssue(raw, s.baseURL), raw, nil
}

func toFieldPayload(fields []FieldInput) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		out = append(out, map[string]any{"name": field.Name, "value": field.Value})
	}
	return out
}

func isFallbackable(err error) bool {
	re, ok := err.(*httpclient.ResponseError)
	return ok && (re.StatusCode == 400 || re.StatusCode == 422)
}

func asMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	if items, ok := v.([]any); ok {
		return items
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
