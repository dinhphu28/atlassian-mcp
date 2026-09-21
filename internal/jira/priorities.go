package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// numericID matches a bare Jira entity id, as opposed to a display name.
var numericID = regexp.MustCompile(`^\d+$`)

// priority is one entry from GET /rest/api/2/priority.
type priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// matchPriority finds the priority referenced by value, which is either a
// numeric priority id or a priority name (matched case-insensitively). The
// error names the priorities this instance actually has, since the set is
// configurable and differs per Jira.
func matchPriority(priorities []priority, value string) (priority, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return priority{}, fmt.Errorf("priority is required, e.g. \"High\"")
	}

	for _, p := range priorities {
		if numericID.MatchString(value) {
			if p.ID == value {
				return p, nil
			}
			continue
		}
		if strings.EqualFold(p.Name, value) {
			return p, nil
		}
	}

	names := make([]string, 0, len(priorities))
	for _, p := range priorities {
		names = append(names, fmt.Sprintf("%s (id %s)", p.Name, p.ID))
	}
	if len(names) == 0 {
		return priority{}, fmt.Errorf("this Jira has no priorities configured")
	}
	return priority{}, fmt.Errorf("unknown priority %q; this Jira has: %s", value, strings.Join(names, ", "))
}

// listPriorities returns the instance's priority catalog.
func (c *Client) listPriorities() ([]priority, error) {
	raw, err := c.get("/rest/api/2/priority")
	if err != nil {
		return nil, err
	}
	var priorities []priority
	if err := json.Unmarshal([]byte(raw), &priorities); err != nil {
		return nil, fmt.Errorf("cannot parse priority catalog: %w", err)
	}
	return priorities, nil
}

// resolvePriority turns a priority name or id into the {"id": …} reference Jira
// expects for a priority field, failing early (with the valid names) instead of
// letting Jira reject an unknown one.
func (c *Client) resolvePriority(value string) (map[string]any, error) {
	priorities, err := c.listPriorities()
	if err != nil {
		return nil, err
	}
	p, err := matchPriority(priorities, value)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": p.ID}, nil
}

// GetPriorities returns the priorities configured on this Jira instance (their
// names and ids are what SetPriority accepts).
func (c *Client) GetPriorities() (string, error) {
	return c.get("/rest/api/2/priority")
}

// SetPriority sets an issue's priority. value is a priority name such as
// "High" (case-insensitive) or a numeric priority id.
func (c *Client) SetPriority(key, value string) (string, error) {
	priorities, err := c.listPriorities()
	if err != nil {
		return "", err
	}
	p, err := matchPriority(priorities, value)
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]any{
		"fields": map[string]any{"priority": map[string]any{"id": p.ID}},
	})
	if _, err := c.do(http.MethodPut, "/rest/api/2/issue/"+url.PathEscape(key), string(body)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Set priority of %s to %s", key, p.Name), nil
}
