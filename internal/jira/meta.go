package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// defaultUserSearchResults is Jira's own page size for a user search, repeated
// here so the caller sees a stable number rather than whatever the instance
// defaults to.
const defaultUserSearchResults = 20

// createMetaPageSize asks for every issue type (or field) in one page: the
// per-issue-type createmeta endpoints are paged, and a project with more issue
// types than the default page holds would otherwise come back truncated.
const createMetaPageSize = 200

// projectSummary is the compact form of one entry from GET /rest/api/2/project.
// Each raw entry carries avatar URL blocks for half a dozen sizes, which dwarf
// the handful of fields that identify the project.
type projectSummary struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	ID             string `json:"id"`
	ProjectTypeKey string `json:"projectTypeKey,omitempty"`
}

// createMetaField is one field of a create screen. The classic createmeta
// payload keys its fields by id inside a map, while the per-issue-type endpoint
// returns them as a list whose elements carry the id in "fieldId", so FieldID
// is populated only in the latter shape.
type createMetaField struct {
	FieldID  string `json:"fieldId"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Schema   struct {
		Type  string `json:"type"`
		Items string `json:"items"`
	} `json:"schema"`
	AllowedValues []map[string]any `json:"allowedValues"`
}

// createMetaProject is one project of a classic createmeta response.
type createMetaProject struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	IssueTypes []struct {
		ID      string                     `json:"id"`
		Name    string                     `json:"name"`
		Subtask bool                       `json:"subtask"`
		Fields  map[string]createMetaField `json:"fields"`
	} `json:"issuetypes"`
}

// CreateMetaField describes one field of a create screen compactly: enough to
// put a value in a jira_create_issue call, without the raw payload's schema and
// operations blocks.
type CreateMetaField struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Type              string   `json:"type,omitempty"`
	AllowedValues     []string `json:"allowed_values,omitempty"`
	AllowedValuesNote string   `json:"allowed_values_note,omitempty"`
}

// CreateMetaIssueType is one issue type a project accepts. The required fields
// are spelled out because they are what a create call must supply; the optional
// ones are listed by id only, since naming them is enough to look them up.
type CreateMetaIssueType struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Subtask          bool              `json:"subtask"`
	RequiredFields   []CreateMetaField `json:"required_fields,omitempty"`
	OptionalFieldIDs []string          `json:"optional_field_ids,omitempty"`
}

// CreateMeta is the compact answer to "what does jira_create_issue have to
// supply for this project". Note carries a caveat when the instance could only
// answer part of the question.
type CreateMeta struct {
	ProjectKey  string                `json:"project_key"`
	ProjectName string                `json:"project_name,omitempty"`
	IssueTypes  []CreateMetaIssueType `json:"issue_types"`
	Note        string                `json:"note,omitempty"`
}

// compactProjects reduces GET /rest/api/2/project to the identifying fields.
func compactProjects(raw string) ([]projectSummary, error) {
	var projects []projectSummary
	if err := json.Unmarshal([]byte(raw), &projects); err != nil {
		return nil, fmt.Errorf("cannot parse project list: %w", err)
	}
	return projects, nil
}

// cappedAllowedValues renders a field's allowed values as labels, keeping at
// most maxAllowedValues of them plus a note saying how many there are: a
// version or component field on a long-lived project lists hundreds.
func cappedAllowedValues(values []map[string]any) (labels []string, note string) {
	for _, value := range values {
		if len(labels) == maxAllowedValues {
			return labels, fmt.Sprintf("showing %d of %d allowed values", maxAllowedValues, len(values))
		}
		if label := allowedValueLabel(value); label != "" {
			labels = append(labels, label)
		}
	}
	return labels, ""
}

// compactCreateFields splits a create screen into the fields that must be
// supplied, described in full, and the ids of the optional rest. Both lists are
// sorted by field id so the same screen always reads the same way.
func compactCreateFields(fields map[string]createMetaField) (required []CreateMetaField, optional []string) {
	ids := make([]string, 0, len(fields))
	for id := range fields {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		f := fields[id]
		if !f.Required {
			optional = append(optional, id)
			continue
		}
		entry := CreateMetaField{
			ID:   id,
			Name: f.Name,
			Type: editFieldType(f.Schema.Type, f.Schema.Items),
		}
		entry.AllowedValues, entry.AllowedValuesNote = cappedAllowedValues(f.AllowedValues)
		required = append(required, entry)
	}
	return required, optional
}

// createFieldsByID keys a list-shaped field payload by its fieldId, so the
// per-issue-type response reaches compactCreateFields the same way the classic
// map-shaped one does.
func createFieldsByID(list []createMetaField) map[string]createMetaField {
	fields := make(map[string]createMetaField, len(list))
	for _, f := range list {
		if f.FieldID == "" {
			continue
		}
		fields[f.FieldID] = f
	}
	return fields
}

// compactCreateMeta reduces a classic createmeta response (expanded down to
// projects.issuetypes.fields) to the project's issue types and their fields.
func compactCreateMeta(raw string) (*CreateMeta, error) {
	var payload struct {
		Projects []createMetaProject `json:"projects"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("cannot parse createmeta response: %w", err)
	}
	if len(payload.Projects) == 0 {
		return nil, fmt.Errorf("createmeta returned no project; check the project key and that you have permission to create issues in it")
	}

	p := payload.Projects[0]
	meta := &CreateMeta{
		ProjectKey:  p.Key,
		ProjectName: p.Name,
		IssueTypes:  make([]CreateMetaIssueType, 0, len(p.IssueTypes)),
	}
	for _, it := range p.IssueTypes {
		entry := CreateMetaIssueType{ID: it.ID, Name: it.Name, Subtask: it.Subtask}
		entry.RequiredFields, entry.OptionalFieldIDs = compactCreateFields(it.Fields)
		meta.IssueTypes = append(meta.IssueTypes, entry)
	}
	return meta, nil
}

// parseCreateMetaIssueTypes reads the paged issue type list served by
// /issue/createmeta/{project}/issuetypes. Fields are not part of that payload;
// they come from the per-issue-type endpoint.
func parseCreateMetaIssueTypes(raw string) ([]CreateMetaIssueType, error) {
	var payload struct {
		Values []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Subtask bool   `json:"subtask"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("cannot parse createmeta issue types: %w", err)
	}

	types := make([]CreateMetaIssueType, 0, len(payload.Values))
	for _, v := range payload.Values {
		types = append(types, CreateMetaIssueType{ID: v.ID, Name: v.Name, Subtask: v.Subtask})
	}
	return types, nil
}

// parseCreateMetaFields reads the paged field list served by
// /issue/createmeta/{project}/issuetypes/{issueTypeId}.
func parseCreateMetaFields(raw string) ([]CreateMetaField, []string, error) {
	var payload struct {
		Values []createMetaField `json:"values"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil, fmt.Errorf("cannot parse createmeta fields: %w", err)
	}

	required, optional := compactCreateFields(createFieldsByID(payload.Values))
	return required, optional, nil
}

// matchCreateMetaIssueType finds the issue type named by value, which is a type
// name (matched case-insensitively) or a numeric type id. The error names the
// types this project accepts, since the set is per-project and a caller cannot
// guess it.
func matchCreateMetaIssueType(types []CreateMetaIssueType, value string) (CreateMetaIssueType, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return CreateMetaIssueType{}, fmt.Errorf("issue_type is required, e.g. \"Task\"")
	}

	for _, it := range types {
		if numericID.MatchString(value) {
			if it.ID == value {
				return it, nil
			}
			continue
		}
		if strings.EqualFold(it.Name, value) {
			return it, nil
		}
	}

	names := make([]string, 0, len(types))
	for _, it := range types {
		names = append(names, fmt.Sprintf("%s (id %s)", it.Name, it.ID))
	}
	if len(names) == 0 {
		return CreateMetaIssueType{}, fmt.Errorf("this project offers no issue types you may create")
	}
	return CreateMetaIssueType{}, fmt.Errorf("unknown issue_type %q; this project accepts: %s", value, strings.Join(names, ", "))
}

// versionPayload builds the body for POST /rest/api/2/version. The dates are
// checked here because Jira answers a malformed one with an opaque 400.
func versionPayload(projectKey, name, description, startDate, releaseDate string, released bool) (map[string]any, error) {
	projectKey = strings.TrimSpace(projectKey)
	name = strings.TrimSpace(name)
	if projectKey == "" {
		return nil, fmt.Errorf("project_key is required, e.g. \"DEV\"")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required, e.g. \"1.4.0\"")
	}
	for label, value := range map[string]string{"start_date": startDate, "release_date": releaseDate} {
		if value != "" && !dateOnly.MatchString(value) {
			return nil, fmt.Errorf("%s must be a date in yyyy-MM-dd format, got %q", label, value)
		}
	}

	payload := map[string]any{
		"project":  projectKey,
		"name":     name,
		"released": released,
	}
	if description != "" {
		payload["description"] = description
	}
	if startDate != "" {
		payload["startDate"] = startDate
	}
	if releaseDate != "" {
		payload["releaseDate"] = releaseDate
	}
	return payload, nil
}

// SearchUsers finds users by a query that Jira matches against the username,
// the display name and the email address. The "name" of a result is the
// username AssignIssue and CreateIssue take; Server/DC has no accountId.
func (c *Client) SearchUsers(query string, maxResults int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required: part of a username, display name or email address")
	}
	if maxResults <= 0 {
		maxResults = defaultUserSearchResults
	}

	q := url.Values{}
	q.Set("username", query)
	q.Set("maxResults", strconv.Itoa(maxResults))
	return c.get("/rest/api/2/user/search?" + q.Encode())
}

// GetMyself returns the user the configured Personal Access Token belongs to.
func (c *Client) GetMyself() (string, error) {
	return c.get("/rest/api/2/myself")
}

// GetProjects lists the projects visible to the token. compact keeps only the
// identifying fields; the full payload is returned verbatim otherwise.
func (c *Client) GetProjects(compact bool) (string, error) {
	raw, err := c.get("/rest/api/2/project")
	if err != nil {
		return "", err
	}
	if !compact {
		return raw, nil
	}

	projects, err := compactProjects(raw)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(projects)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GetProjectVersions returns a project's versions (the names jira_set_fields
// accepts for fix_versions / affects_versions).
func (c *Client) GetProjectVersions(projectKey string) (string, error) {
	if strings.TrimSpace(projectKey) == "" {
		return "", fmt.Errorf("project_key is required, e.g. \"DEV\"")
	}
	return c.get("/rest/api/2/project/" + url.PathEscape(projectKey) + "/versions")
}

// GetProjectComponents returns a project's components (the names
// jira_set_fields accepts for components).
func (c *Client) GetProjectComponents(projectKey string) (string, error) {
	if strings.TrimSpace(projectKey) == "" {
		return "", fmt.Errorf("project_key is required, e.g. \"DEV\"")
	}
	return c.get("/rest/api/2/project/" + url.PathEscape(projectKey) + "/components")
}

// GetFilters returns the caller's favourite filters, or a single filter when
// filterID is given. A filter's "jql" is what Search runs.
func (c *Client) GetFilters(filterID string) (string, error) {
	if filterID = strings.TrimSpace(filterID); filterID != "" {
		return c.get("/rest/api/2/filter/" + url.PathEscape(filterID))
	}
	return c.get("/rest/api/2/filter/favourite")
}

// createMetaTypesPath is the base of the per-issue-type createmeta endpoints
// that Jira 8.4 introduced.
func createMetaTypesPath(projectKey string) string {
	return "/rest/api/2/issue/createmeta/" + url.PathEscape(projectKey) + "/issuetypes"
}

// GetCreateMeta reports what a create call must supply for a project: the issue
// types it accepts and, per type, the fields its create screen requires.
// issueType narrows the answer to one type. raw returns Jira's own payload.
//
// The single createmeta resource was removed in Jira 9 (it was notorious for
// oversized responses), so a failure there falls back to the per-issue-type
// endpoints Jira 8.4 added.
func (c *Client) GetCreateMeta(projectKey, issueType string, raw bool) (string, error) {
	projectKey = strings.TrimSpace(projectKey)
	issueType = strings.TrimSpace(issueType)
	if projectKey == "" {
		return "", fmt.Errorf("project_key is required, e.g. \"DEV\"")
	}

	body, classicErr := c.get("/rest/api/2/issue/createmeta?" + createMetaQuery(projectKey, issueType, true).Encode())
	if classicErr != nil {
		return c.createMetaByIssueType(projectKey, issueType, raw, classicErr)
	}
	if raw {
		return body, nil
	}

	meta, err := compactCreateMeta(body)
	if err != nil {
		return "", err
	}
	// Jira filters by issue type server-side and simply returns none when the
	// name or id is wrong, so re-read the unfiltered list to name the valid ones.
	if issueType != "" && len(meta.IssueTypes) == 0 {
		return "", c.unknownIssueTypeError(projectKey, issueType)
	}
	return createMetaJSON(meta)
}

// createMetaQuery builds the query for the classic createmeta resource. The
// issue type filter is applied server-side by the endpoint, which takes names
// and ids through two different parameters: sending a numeric id as a name
// matches nothing, so a bare number is routed to issuetypeIds instead.
func createMetaQuery(projectKey, issueType string, expandFields bool) url.Values {
	q := url.Values{}
	q.Set("projectKeys", projectKey)
	if expandFields {
		q.Set("expand", "projects.issuetypes.fields")
	}
	if issueType = strings.TrimSpace(issueType); issueType != "" {
		if numericID.MatchString(issueType) {
			q.Set("issuetypeIds", issueType)
		} else {
			q.Set("issuetypeNames", issueType)
		}
	}
	return q
}

// unknownIssueTypeError re-reads the project's issue types so an unknown
// issue_type fails with the list of names that would have worked.
func (c *Client) unknownIssueTypeError(projectKey, issueType string) error {
	body, err := c.get("/rest/api/2/issue/createmeta?" + createMetaQuery(projectKey, "", false).Encode())
	if err != nil {
		return fmt.Errorf("no issue type %q in project %s", issueType, projectKey)
	}
	meta, err := compactCreateMeta(body)
	if err != nil {
		return fmt.Errorf("no issue type %q in project %s", issueType, projectKey)
	}
	_, matchErr := matchCreateMetaIssueType(meta.IssueTypes, issueType)
	if matchErr == nil {
		return fmt.Errorf("no create screen for issue type %q in project %s", issueType, projectKey)
	}
	return matchErr
}

// createMetaByIssueType answers GetCreateMeta through the endpoints that
// replaced the removed createmeta resource. Fields are fetched only when an
// issue type is named: that endpoint serves one issue type at a time, so
// describing every type would be one request each.
func (c *Client) createMetaByIssueType(projectKey, issueType string, raw bool, classicErr error) (string, error) {
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(createMetaPageSize))

	listBody, err := c.get(createMetaTypesPath(projectKey) + "?" + q.Encode())
	if err != nil {
		return "", fmt.Errorf("createmeta failed (%v) and the per-issue-type endpoint failed too: %w", classicErr, err)
	}

	types, err := parseCreateMetaIssueTypes(listBody)
	if err != nil {
		return "", err
	}

	if issueType == "" {
		if raw {
			return listBody, nil
		}
		return createMetaJSON(&CreateMeta{
			ProjectKey: projectKey,
			IssueTypes: types,
			Note:       "this Jira serves createmeta one issue type at a time; call again with issue_type to see that type's fields",
		})
	}

	it, err := matchCreateMetaIssueType(types, issueType)
	if err != nil {
		return "", err
	}

	fieldsBody, err := c.get(createMetaTypesPath(projectKey) + "/" + url.PathEscape(it.ID) + "?" + q.Encode())
	if err != nil {
		return "", err
	}
	if raw {
		return fieldsBody, nil
	}

	required, optional, err := parseCreateMetaFields(fieldsBody)
	if err != nil {
		return "", err
	}
	it.RequiredFields, it.OptionalFieldIDs = required, optional

	return createMetaJSON(&CreateMeta{
		ProjectKey: projectKey,
		IssueTypes: []CreateMetaIssueType{it},
	})
}

// createMetaJSON renders a compact createmeta projection as the JSON string the
// read methods return.
func createMetaJSON(meta *CreateMeta) (string, error) {
	out, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CreateVersion creates a project version (a release). startDate and
// releaseDate are yyyy-MM-dd and optional; released marks the version released
// at creation time. The created version's JSON carries the id other version
// APIs need.
func (c *Client) CreateVersion(projectKey, name, description, startDate, releaseDate string, released bool) (string, error) {
	payload, err := versionPayload(projectKey, name, description, startDate, releaseDate, released)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return c.do(http.MethodPost, "/rest/api/2/version", string(body))
}
