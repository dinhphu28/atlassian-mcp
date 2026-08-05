package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Advanced Roadmaps identifies its baseline (Target start/end) fields by a
// stable schema "custom" key, regardless of the instance-specific
// customfield_NNNNN id or the localized display name.
const (
	targetStartCustomKey = "jpo-custom-field-baseline-start"
	targetEndCustomKey   = "jpo-custom-field-baseline-end"
)

// dateOnly matches Jira's yyyy-MM-dd date-field format.
var dateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// field is one entry from GET /rest/api/2/field.
type field struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Schema struct {
		Type   string `json:"type"`
		Custom string `json:"custom"`
	} `json:"schema"`
}

// fieldIDByCustomKey returns the id of the field whose schema custom key ends
// with suffix (e.g. "jpo-custom-field-baseline-start"). The full key is prefixed
// by a plugin namespace (com.atlassian.jpo:...), so it is matched by suffix.
func (c *Client) fieldIDByCustomKey(fields []field, suffix string) (string, bool) {
	for _, f := range fields {
		if strings.HasSuffix(f.Schema.Custom, suffix) {
			return f.ID, true
		}
	}
	return "", false
}

// listFields returns the instance's field catalog.
func (c *Client) listFields() ([]field, error) {
	raw, err := c.get("/rest/api/2/field")
	if err != nil {
		return nil, err
	}
	var fields []field
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, fmt.Errorf("cannot parse field catalog: %w", err)
	}
	return fields, nil
}

// SetTargetDates sets an issue's Advanced Roadmaps "Target start" and/or
// "Target end" dates (yyyy-MM-dd). Empty values are left unchanged; at least one
// must be provided. The instance-specific customfield ids are discovered from
// the field catalog by their stable Roadmaps schema key.
func (c *Client) SetTargetDates(key, targetStart, targetEnd string) (string, error) {
	if targetStart == "" && targetEnd == "" {
		return "", fmt.Errorf("provide 'target_start' and/or 'target_end'")
	}
	for label, v := range map[string]string{"target_start": targetStart, "target_end": targetEnd} {
		if v != "" && !dateOnly.MatchString(v) {
			return "", fmt.Errorf("%s must be a date in yyyy-MM-dd format, got %q", label, v)
		}
	}

	fields, err := c.listFields()
	if err != nil {
		return "", err
	}

	payload := map[string]any{}
	if targetStart != "" {
		id, ok := c.fieldIDByCustomKey(fields, targetStartCustomKey)
		if !ok {
			return "", fmt.Errorf("this Jira has no 'Target start' field (Advanced Roadmaps not enabled?)")
		}
		payload[id] = targetStart
	}
	if targetEnd != "" {
		id, ok := c.fieldIDByCustomKey(fields, targetEndCustomKey)
		if !ok {
			return "", fmt.Errorf("this Jira has no 'Target end' field (Advanced Roadmaps not enabled?)")
		}
		payload[id] = targetEnd
	}

	body, _ := json.Marshal(map[string]any{"fields": payload})
	if _, err := c.do(http.MethodPut, "/rest/api/2/issue/"+url.PathEscape(key), string(body)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Set target dates on %s", key), nil
}
