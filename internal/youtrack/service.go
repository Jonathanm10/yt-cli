package youtrack

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Jonathanm10/yt-cli/internal/httpclient"
	"github.com/Jonathanm10/yt-cli/internal/output"
)

const issueFields = "id,idReadable,summary,description,project(id,shortName,name),customFields(id,name,$type,value(id,login,name,presentation,text)),attachments(id,name,url),updated"
const projectFields = "id,shortName,name"
const projectCustomFieldFields = "id,isPublic,field(name),$type"

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

type projectRef struct {
	ID        string
	ShortName string
}

type projectCustomField struct {
	Name     string
	Type     string
	IsPublic bool
}

type supportedFieldType struct {
	IssueType string
	ValueKey  string
}

var supportedFieldTypes = map[string]supportedFieldType{
	"EnumProjectCustomField":  {IssueType: "SingleEnumIssueCustomField", ValueKey: "name"},
	"StateProjectCustomField": {IssueType: "StateIssueCustomField", ValueKey: "name"},
	"TextProjectCustomField":  {IssueType: "TextIssueCustomField", ValueKey: "text"},
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
	body := map[string]any{"summary": in.Summary}
	if len(in.Fields) > 0 {
		project, err := s.resolveProjectRef(ctx, in.Project)
		if err != nil {
			return nil, nil, err
		}
		customFields, err := s.resolveTypedCustomFields(ctx, project, in.Fields)
		if err != nil {
			return nil, nil, err
		}
		body["project"] = map[string]any{"shortName": project.ShortName}
		body["customFields"] = customFields
	} else {
		body["project"] = map[string]any{"shortName": in.Project}
	}
	if strings.TrimSpace(in.Description) != "" {
		body["description"] = in.Description
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
		project, err := s.resolveIssueProject(ctx, in.ID)
		if err != nil {
			return nil, nil, err
		}
		customFields, err := s.resolveTypedCustomFields(ctx, project, in.Fields)
		if err != nil {
			return nil, nil, err
		}
		body["customFields"] = customFields
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

func (s *Service) resolveProjectRef(ctx context.Context, project string) (projectRef, error) {
	query := url.Values{"fields": []string{projectFields}, "query": []string{strings.TrimSpace(project)}}
	payload, err := s.client.DoJSON(ctx, "GET", "/api/admin/projects", query, nil)
	if err != nil {
		return projectRef{}, err
	}
	target := normalizeFieldKey(project)
	for _, item := range asSlice(payload) {
		raw := asMap(item)
		shortName := fmt.Sprint(raw["shortName"])
		id := fmt.Sprint(raw["id"])
		name := fmt.Sprint(raw["name"])
		if normalizeFieldKey(shortName) == target || normalizeFieldKey(id) == target || normalizeFieldKey(name) == target {
			return projectRef{ID: id, ShortName: shortName}, nil
		}
	}
	return projectRef{}, output.NewError(2, "validation_error", fmt.Sprintf("could not resolve project %q for custom-field metadata", strings.TrimSpace(project)), map[string]any{
		"project": strings.TrimSpace(project),
		"stage":   "project_resolution",
	})
}

func (s *Service) resolveIssueProject(ctx context.Context, issueID string) (projectRef, error) {
	payload, err := s.client.DoJSON(ctx, "GET", "/api/issues/"+url.PathEscape(issueID), url.Values{"fields": []string{"project(id,shortName,name)"}}, nil)
	if err != nil {
		return projectRef{}, err
	}
	project := asMap(asMap(payload)["project"])
	return projectRef{ID: fmt.Sprint(project["id"]), ShortName: fmt.Sprint(project["shortName"])}, nil
}

func (s *Service) resolveTypedCustomFields(ctx context.Context, project projectRef, fields []FieldInput) ([]map[string]any, error) {
	metadata, err := s.listProjectCustomFields(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		meta, ok := metadata[normalizeFieldKey(field.Name)]
		if !ok {
			return nil, fieldError(field, "metadata_lookup", fmt.Sprintf("custom field %q is not available in project %s or its metadata is not visible", strings.TrimSpace(field.Name), project.ShortNameOrID()), map[string]any{
				"projectId":        project.ID,
				"projectShortName": project.ShortName,
			})
		}
		if !meta.IsPublic {
			return nil, fieldError(field, "metadata_visibility_permission", fmt.Sprintf("custom field %q in project %s requires private-field metadata access", meta.Name, project.ShortNameOrID()), map[string]any{
				"projectId":        project.ID,
				"projectShortName": project.ShortName,
				"fieldType":        meta.Type,
			})
		}
		spec, ok := supportedFieldTypes[meta.Type]
		if !ok {
			return nil, fieldError(field, "unsupported_class", fmt.Sprintf("custom field %q uses unsupported project field type %q", meta.Name, meta.Type), map[string]any{
				"projectId":        project.ID,
				"projectShortName": project.ShortName,
				"fieldType":        meta.Type,
			})
		}
		out = append(out, map[string]any{
			"name":  meta.Name,
			"$type": spec.IssueType,
			"value": map[string]any{spec.ValueKey: field.Value},
		})
	}
	return out, nil
}

func (s *Service) listProjectCustomFields(ctx context.Context, project projectRef) (map[string]projectCustomField, error) {
	payload, err := s.client.DoJSON(ctx, "GET", "/api/admin/projects/"+url.PathEscape(project.ID)+"/customFields", url.Values{"fields": []string{projectCustomFieldFields}}, nil)
	if err != nil {
		var responseErr *httpclient.ResponseError
		if errors.As(err, &responseErr) && (responseErr.StatusCode == 401 || responseErr.StatusCode == 403) {
			details := map[string]any{
				"projectId":        project.ID,
				"projectShortName": project.ShortName,
				"stage":            "metadata_visibility_permission",
				"status":           responseErr.StatusCode,
			}
			if responseErr.Payload != nil {
				details["response"] = responseErr.Payload
			}
			return nil, output.NewError(2, "metadata_visibility_error", fmt.Sprintf("token cannot read custom-field metadata for project %s", project.ShortNameOrID()), details)
		}
		return nil, err
	}
	out := make(map[string]projectCustomField, len(asSlice(payload)))
	for _, item := range asSlice(payload) {
		raw := asMap(item)
		field := asMap(raw["field"])
		name := strings.TrimSpace(fmt.Sprint(field["name"]))
		if name == "" {
			continue
		}
		out[normalizeFieldKey(name)] = projectCustomField{Name: name, Type: strings.TrimSpace(fmt.Sprint(raw["$type"])), IsPublic: asBool(raw["isPublic"])}
	}
	return out, nil
}

func (p projectRef) ShortNameOrID() string {
	if strings.TrimSpace(p.ShortName) != "" && p.ShortName != "<nil>" {
		return p.ShortName
	}
	return strings.TrimSpace(p.ID)
}

func fieldError(field FieldInput, stage, message string, extra map[string]any) error {
	details := map[string]any{"field": strings.TrimSpace(field.Name), "value": field.Value, "stage": stage}
	for key, value := range extra {
		details[key] = value
	}
	code := "validation_error"
	switch stage {
	case "unsupported_class":
		code = "unsupported_field_type"
	case "metadata_lookup", "metadata_visibility_permission":
		code = "metadata_visibility_error"
	}
	return output.NewError(2, code, message, details)
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

func normalizeFieldKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func asBool(v any) bool {
	if value, ok := v.(bool); ok {
		return value
	}
	return false
}
