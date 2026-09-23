package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DeleteIssue permanently deletes an issue. Jira refuses the delete outright
// when the issue has sub-tasks, unless deleteSubtasks is true — in which case
// the sub-tasks are deleted along with it.
func (c *Client) DeleteIssue(key string, deleteSubtasks bool) error {
	if key == "" {
		return fmt.Errorf("issue_key is required")
	}
	q := url.Values{}
	q.Set("deleteSubtasks", strconv.FormatBool(deleteSubtasks))

	_, err := c.do(http.MethodDelete, "/rest/api/2/issue/"+url.PathEscape(key)+"?"+q.Encode(), "")
	return err
}

// DeleteAttachment removes an attachment by its id (the ids are in an issue's
// "attachment" field, from GetIssue). The id identifies the attachment
// globally, so the issue key is not part of the path.
func (c *Client) DeleteAttachment(attachmentID string) error {
	if attachmentID == "" {
		return fmt.Errorf("attachment_id is required")
	}
	_, err := c.do(http.MethodDelete, "/rest/api/2/attachment/"+url.PathEscape(attachmentID), "")
	return err
}

// changelogEntry is one entry of changelog.histories as Jira returns it. The
// value fields are pointers because Jira sends null for the side of a change
// that did not exist (a field being set for the first time, or cleared).
type changelogEntry struct {
	Created string `json:"created"`
	Author  struct {
		DisplayName string `json:"displayName"`
		Name        string `json:"name"`
	} `json:"author"`
	Items []struct {
		Field      string  `json:"field"`
		From       *string `json:"from"`
		FromString *string `json:"fromString"`
		To         *string `json:"to"`
		ToString   *string `json:"toString"`
	} `json:"items"`
}

// changeValue picks the human-readable side of a changelog item, falling back
// to the raw id when Jira has no display string for it, and staying null when
// there was no value at all.
func changeValue(text, id *string) any {
	if text != nil {
		return *text
	}
	if id != nil {
		return *id
	}
	return nil
}

// compactChangelog projects Jira's verbose changelog down to who changed what
// and when: one entry per change, each carrying only the display name, the
// timestamp and the changed fields. Jira returns histories oldest first and
// that order is kept, so the newest change reads last; a positive limit keeps
// only that many of the most recent entries.
func compactChangelog(entries []changelogEntry, limit int) []map[string]any {
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		items := make([]map[string]any, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, map[string]any{
				"field": it.Field,
				"from":  changeValue(it.FromString, it.From),
				"to":    changeValue(it.ToString, it.To),
			})
		}

		author := e.Author.DisplayName
		if author == "" {
			author = e.Author.Name
		}

		out = append(out, map[string]any{
			"created": e.Created,
			"author":  author,
			"items":   items,
		})
	}
	return out
}

// GetIssueHistory returns an issue's change history. Server/DC has no separate
// changelog resource, so the history is read off the issue itself with
// expand=changelog, with the fields narrowed to the summary because only the
// changelog is wanted. raw returns Jira's payload untouched; otherwise the
// histories are compacted by compactChangelog, keeping at most limit of the
// most recent entries.
func (c *Client) GetIssueHistory(key string, limit int, raw bool) (string, error) {
	body, err := c.get("/rest/api/2/issue/" + url.PathEscape(key) + "?expand=changelog&fields=summary")
	if err != nil {
		return "", err
	}
	if raw {
		return body, nil
	}

	var issue struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
		Changelog struct {
			Total     int              `json:"total"`
			Histories []changelogEntry `json:"histories"`
		} `json:"changelog"`
	}
	if err := json.Unmarshal([]byte(body), &issue); err != nil {
		return "", fmt.Errorf("cannot parse the changelog of %s: %w", key, err)
	}

	out, err := json.Marshal(map[string]any{
		"key":     issue.Key,
		"summary": issue.Fields.Summary,
		"total":   issue.Changelog.Total,
		"entries": compactChangelog(issue.Changelog.Histories, limit),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// bulkIssueUpdates turns a JSON array of issue field objects into the
// "issueUpdates" list Jira's bulk endpoint expects. An element that already
// carries a top-level "fields" key is passed through untouched, so a caller
// who copied Jira's own wire shape is not double-wrapped.
func bulkIssueUpdates(issuesJSON string) ([]map[string]any, error) {
	issuesJSON = strings.TrimSpace(issuesJSON)
	if issuesJSON == "" {
		return nil, fmt.Errorf("issues_json is required: a JSON array of issue field objects")
	}

	var elements []json.RawMessage
	if err := json.Unmarshal([]byte(issuesJSON), &elements); err != nil {
		return nil, fmt.Errorf("issues_json must be a JSON array of issue field objects: %w", err)
	}
	if len(elements) == 0 {
		return nil, fmt.Errorf("issues_json must contain at least one issue")
	}

	updates := make([]map[string]any, 0, len(elements))
	for i, element := range elements {
		var obj map[string]any
		if err := json.Unmarshal(element, &obj); err != nil {
			return nil, fmt.Errorf("issues_json[%d] must be a JSON object: %w", i, err)
		}
		if obj == nil {
			return nil, fmt.Errorf("issues_json[%d] must be a JSON object, not null", i)
		}
		if _, ok := obj["fields"]; ok {
			updates = append(updates, obj)
			continue
		}
		updates = append(updates, map[string]any{"fields": obj})
	}
	return updates, nil
}

// BulkCreateIssues creates several issues in one request. issuesJSON is a JSON
// array whose elements are each the fields object CreateIssue would build.
// Jira reports the outcome per issue, so partial success is normal: the
// response carries both the created issues and the per-element errors. Jira
// only fails the whole request when every element is rejected.
func (c *Client) BulkCreateIssues(issuesJSON string) (string, error) {
	updates, err := bulkIssueUpdates(issuesJSON)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]any{"issueUpdates": updates})
	if err != nil {
		return "", err
	}
	return c.do(http.MethodPost, "/rest/api/2/issue/bulk", string(body))
}
