package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// normalizeFields cleans a comma-separated field selector for the search API's
// "fields" parameter, dropping blanks so a trailing comma or a stray space does
// not become an empty field name Jira rejects.
func normalizeFields(fields string) string {
	parts := strings.Split(fields, ",")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ",")
}

// Search runs a JQL query and returns up to limit issues, starting at startAt
// (0-based) so callers can page. fields is an optional comma-separated selector
// passed to the API verbatim; empty means Jira's default (every navigable
// field), which is by far the most expensive option.
func (c *Client) Search(jql string, limit, startAt int, fields string) (string, error) {
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("maxResults", strconv.Itoa(limit))
	if startAt > 0 {
		q.Set("startAt", strconv.Itoa(startAt))
	}
	if f := normalizeFields(fields); f != "" {
		q.Set("fields", f)
	}
	return c.get("/rest/api/2/search?" + q.Encode())
}

// GetIssue returns a single issue by key or ID.
func (c *Client) GetIssue(key string) (string, error) {
	return c.get("/rest/api/2/issue/" + url.PathEscape(key))
}

// GetComments returns the comments on an issue.
func (c *Client) GetComments(key string) (string, error) {
	return c.get("/rest/api/2/issue/" + url.PathEscape(key) + "/comment")
}

// CreateIssue creates an issue in the given project. description is optional and
// uses Jira wiki markup. parentKey is optional and required only for sub-task
// issue types, which Jira rejects unless a parent is supplied at creation time.
// assignee is an optional Jira username to assign the new issue to, and
// priority an optional priority name or id (Jira's default applies if empty).
func (c *Client) CreateIssue(projectKey, issueType, summary, description, parentKey, assignee, priority string) (string, error) {
	fields := map[string]any{
		"project":   map[string]any{"key": projectKey},
		"issuetype": map[string]any{"name": issueType},
		"summary":   summary,
	}
	if description != "" {
		fields["description"] = description
	}
	if parentKey != "" {
		fields["parent"] = map[string]any{"key": parentKey}
	}
	if assignee != "" {
		fields["assignee"] = map[string]any{"name": assignee}
	}
	if priority != "" {
		ref, err := c.resolvePriority(priority)
		if err != nil {
			return "", err
		}
		fields["priority"] = ref
	}

	body, _ := json.Marshal(map[string]any{"fields": fields})
	return c.do(http.MethodPost, "/rest/api/2/issue", string(body))
}

// commentVisibility builds the "visibility" object restricting a comment to a
// project role or a group. Both parts are meaningless alone, so supplying one
// without the other is an error rather than a silently public comment.
func commentVisibility(visibilityType, visibilityValue string) (map[string]any, error) {
	visibilityType = strings.ToLower(strings.TrimSpace(visibilityType))
	visibilityValue = strings.TrimSpace(visibilityValue)

	switch {
	case visibilityType == "" && visibilityValue == "":
		return nil, nil
	case visibilityType == "":
		return nil, fmt.Errorf("visibility_value %q needs visibility_type (\"role\" or \"group\")", visibilityValue)
	case visibilityValue == "":
		return nil, fmt.Errorf("visibility_type %q needs visibility_value, e.g. the role or group name", visibilityType)
	case visibilityType != "role" && visibilityType != "group":
		return nil, fmt.Errorf("unknown visibility_type %q (use \"role\" or \"group\")", visibilityType)
	}

	return map[string]any{"type": visibilityType, "value": visibilityValue}, nil
}

// AddComment posts a comment (Jira wiki markup) on an issue. visibilityType is
// "role" or "group" and visibilityValue the role or group name; both empty
// posts a comment everyone who can see the issue can read.
func (c *Client) AddComment(key, body, visibilityType, visibilityValue string) (string, error) {
	visibility, err := commentVisibility(visibilityType, visibilityValue)
	if err != nil {
		return "", err
	}

	payload := map[string]any{"body": body}
	if visibility != nil {
		payload["visibility"] = visibility
	}

	raw, _ := json.Marshal(payload)
	return c.do(http.MethodPost, "/rest/api/2/issue/"+url.PathEscape(key)+"/comment", string(raw))
}

// commentVisibilityOf reads the restriction currently on a comment, so an edit
// can put it back. A comment nobody restricted yields nil.
func (c *Client) commentVisibilityOf(key, commentID string) (map[string]any, error) {
	raw, err := c.get("/rest/api/2/issue/" + url.PathEscape(key) + "/comment/" + url.PathEscape(commentID))
	if err != nil {
		return nil, fmt.Errorf("cannot read comment %s on %s, which is needed so the edit keeps its "+
			"visibility restriction: %w", commentID, key, err)
	}

	var comment struct {
		Visibility struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"visibility"`
	}
	if err := json.Unmarshal([]byte(raw), &comment); err != nil {
		return nil, fmt.Errorf("cannot parse comment %s on %s: %w", commentID, key, err)
	}
	if comment.Visibility.Type == "" || comment.Visibility.Value == "" {
		return nil, nil
	}
	return map[string]any{"type": comment.Visibility.Type, "value": comment.Visibility.Value}, nil
}

// UpdateComment edits an existing comment (Jira wiki markup) on an issue.
//
// The PUT replaces the comment, so a body sent without a "visibility" object
// publishes a role- or group-restricted comment to everyone who can see the
// issue. visibilityType and visibilityValue set a new restriction; both empty
// keeps whatever the comment already has, read back from Jira first; a
// visibilityType of "none" deliberately drops the restriction.
func (c *Client) UpdateComment(key, commentID, body, visibilityType, visibilityValue string) (string, error) {
	payload := map[string]any{"body": body}

	if strings.EqualFold(strings.TrimSpace(visibilityType), "none") {
		// An explicit request to publish the comment: send no visibility.
	} else {
		visibility, err := commentVisibility(visibilityType, visibilityValue)
		if err != nil {
			return "", err
		}
		if visibility == nil {
			if visibility, err = c.commentVisibilityOf(key, commentID); err != nil {
				return "", err
			}
		}
		if visibility != nil {
			payload["visibility"] = visibility
		}
	}

	raw, _ := json.Marshal(payload)
	return c.do(http.MethodPut,
		"/rest/api/2/issue/"+url.PathEscape(key)+"/comment/"+url.PathEscape(commentID),
		string(raw))
}

// DeleteComment deletes a comment from an issue.
func (c *Client) DeleteComment(key, commentID string) error {
	_, err := c.do(http.MethodDelete,
		"/rest/api/2/issue/"+url.PathEscape(key)+"/comment/"+url.PathEscape(commentID), "")
	return err
}

// AssignIssue sets an issue's assignee. assignee is a Jira username; an empty
// string or "null" unassigns the issue, and "-1" resets it to the project's
// default assignee.
func (c *Client) AssignIssue(key, assignee string) error {
	var name any
	switch assignee {
	case "", "null":
		name = nil
	default:
		name = assignee
	}
	body, _ := json.Marshal(map[string]any{"name": name})
	_, err := c.do(http.MethodPut, "/rest/api/2/issue/"+url.PathEscape(key)+"/assignee", string(body))
	return err
}

// UpdateIssue updates an issue's summary and/or description. Empty values are
// left unchanged; at least one must be provided.
func (c *Client) UpdateIssue(key, summary, description string) error {
	fields := map[string]any{}
	if summary != "" {
		fields["summary"] = summary
	}
	if description != "" {
		fields["description"] = description
	}

	body, _ := json.Marshal(map[string]any{"fields": fields})
	_, err := c.do(http.MethodPut, "/rest/api/2/issue/"+url.PathEscape(key), string(body))
	return err
}

// GetTransitions lists the status transitions available for an issue (their ids
// are used with TransitionIssue). expandFields also returns each transition's
// screen fields, which is how to discover what a transition requires before
// attempting it.
func (c *Client) GetTransitions(key string, expandFields bool) (string, error) {
	path := "/rest/api/2/issue/" + url.PathEscape(key) + "/transitions"
	if expandFields {
		path += "?expand=transitions.fields"
	}
	return c.get(path)
}

// transitionPayload builds the body of a transition request. resolutionName is
// an already-resolved resolution name, comment optional Jira wiki markup, and
// extraFields an optional JSON object of any other field the transition screen
// requires. Fields and the comment travel differently: a comment is an
// "update" operation, everything else a straight "fields" value.
func transitionPayload(transitionID, resolutionName, comment, extraFields string) (map[string]any, error) {
	fields := map[string]any{}
	if extraFields = strings.TrimSpace(extraFields); extraFields != "" {
		if err := json.Unmarshal([]byte(extraFields), &fields); err != nil {
			return nil, fmt.Errorf("fields must be a JSON object such as {\"customfield_10001\": \"value\"}: %w", err)
		}
	}
	if resolutionName != "" {
		fields["resolution"] = map[string]any{"name": resolutionName}
	}

	payload := map[string]any{
		"transition": map[string]any{"id": transitionID},
	}
	if len(fields) > 0 {
		payload["fields"] = fields
	}
	if comment != "" {
		payload["update"] = map[string]any{
			"comment": []any{map[string]any{"add": map[string]any{"body": comment}}},
		}
	}

	return payload, nil
}

// TransitionIssue moves an issue through the transition with the given id.
// resolution, comment and fields are optional and cover transition screens that
// demand input: resolution is a resolution name (validated against the
// instance's catalog), comment is Jira wiki markup, and fields is a raw JSON
// object merged into the request's "fields" for anything else the screen
// requires. Discover what a transition asks for with GetTransitions(key, true).
func (c *Client) TransitionIssue(key, transitionID, resolution, comment, fields string) error {
	resolutionName := ""
	if strings.TrimSpace(resolution) != "" {
		r, err := c.resolveResolution(resolution)
		if err != nil {
			return err
		}
		resolutionName = r
	}

	payload, err := transitionPayload(transitionID, resolutionName, comment, fields)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(payload)
	_, err = c.do(http.MethodPost, "/rest/api/2/issue/"+url.PathEscape(key)+"/transitions", string(body))
	return err
}

// resolution is one entry from GET /rest/api/2/resolution.
type resolution struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// matchResolution finds the resolution referenced by value, either a numeric
// resolution id or a name matched case-insensitively. Like priorities, the set
// is per-instance, so the error names the ones this Jira actually has.
func matchResolution(resolutions []resolution, value string) (resolution, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return resolution{}, fmt.Errorf("resolution is required, e.g. \"Done\"")
	}

	for _, r := range resolutions {
		if numericID.MatchString(value) {
			if r.ID == value {
				return r, nil
			}
			continue
		}
		if strings.EqualFold(r.Name, value) {
			return r, nil
		}
	}

	names := make([]string, 0, len(resolutions))
	for _, r := range resolutions {
		names = append(names, fmt.Sprintf("%s (id %s)", r.Name, r.ID))
	}
	if len(names) == 0 {
		return resolution{}, fmt.Errorf("this Jira has no resolutions configured")
	}
	return resolution{}, fmt.Errorf("unknown resolution %q; this Jira has: %s", value, strings.Join(names, ", "))
}

// resolveResolution turns a resolution name or id into the canonical name Jira
// expects on a transition screen, failing with the valid names instead of
// letting the transition come back as an opaque "field is required" 400.
func (c *Client) resolveResolution(value string) (string, error) {
	raw, err := c.get("/rest/api/2/resolution")
	if err != nil {
		return "", err
	}
	var resolutions []resolution
	if err := json.Unmarshal([]byte(raw), &resolutions); err != nil {
		return "", fmt.Errorf("cannot parse resolution catalog: %w", err)
	}

	r, err := matchResolution(resolutions, value)
	if err != nil {
		return "", err
	}
	return r.Name, nil
}
