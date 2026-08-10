package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Search runs a JQL query and returns up to limit issues.
func (c *Client) Search(jql string, limit int) (string, error) {
	path := fmt.Sprintf("/rest/api/2/search?maxResults=%d&jql=%s", limit, url.QueryEscape(jql))
	return c.get(path)
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
func (c *Client) CreateIssue(projectKey, issueType, summary, description, parentKey string) (string, error) {
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

	body, _ := json.Marshal(map[string]any{"fields": fields})
	return c.do(http.MethodPost, "/rest/api/2/issue", string(body))
}

// AddComment posts a comment (Jira wiki markup) on an issue.
func (c *Client) AddComment(key, body string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"body": body})
	return c.do(http.MethodPost, "/rest/api/2/issue/"+url.PathEscape(key)+"/comment", string(payload))
}

// UpdateComment edits an existing comment (Jira wiki markup) on an issue.
func (c *Client) UpdateComment(key, commentID, body string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"body": body})
	return c.do(http.MethodPut,
		"/rest/api/2/issue/"+url.PathEscape(key)+"/comment/"+url.PathEscape(commentID),
		string(payload))
}

// DeleteComment deletes a comment from an issue.
func (c *Client) DeleteComment(key, commentID string) error {
	_, err := c.do(http.MethodDelete,
		"/rest/api/2/issue/"+url.PathEscape(key)+"/comment/"+url.PathEscape(commentID), "")
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
// are used with TransitionIssue).
func (c *Client) GetTransitions(key string) (string, error) {
	return c.get("/rest/api/2/issue/" + url.PathEscape(key) + "/transitions")
}

// TransitionIssue moves an issue through the transition with the given id.
func (c *Client) TransitionIssue(key, transitionID string) error {
	body, _ := json.Marshal(map[string]any{
		"transition": map[string]any{"id": transitionID},
	})
	_, err := c.do(http.MethodPost, "/rest/api/2/issue/"+url.PathEscape(key)+"/transitions", string(body))
	return err
}
