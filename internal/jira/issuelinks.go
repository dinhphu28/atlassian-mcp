package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// issueLinkType is one entry from GET /rest/api/2/issueLinkType. Inward and
// Outward are the human-readable descriptions of each direction, e.g. "is
// blocked by" and "blocks" for the "Blocks" type.
type issueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// matchIssueLinkType finds the link type referenced by value, which is a type
// name ("Blocks"), a numeric type id, or either direction's description
// ("blocks", "is blocked by"), all matched case-insensitively.
//
// swap reports that value named the inward direction, so the issue the caller
// put first is the inward side of the link: "DEV-1 is blocked by DEV-2" links
// the same pair as "DEV-2 blocks DEV-1", with the ends reversed.
func matchIssueLinkType(types []issueLinkType, value string) (t issueLinkType, swap bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return issueLinkType{}, false, fmt.Errorf("link_type is required, e.g. \"Blocks\" or \"is blocked by\"")
	}

	// A name or id wins over a direction description, so a type whose name
	// collides with another type's description still resolves to itself.
	for _, lt := range types {
		if strings.EqualFold(lt.Name, value) || lt.ID == value {
			return lt, false, nil
		}
	}
	for _, lt := range types {
		if strings.EqualFold(lt.Outward, value) {
			return lt, false, nil
		}
		if strings.EqualFold(lt.Inward, value) {
			return lt, true, nil
		}
	}

	descriptions := make([]string, 0, len(types))
	for _, lt := range types {
		descriptions = append(descriptions, fmt.Sprintf("%s (%q / %q)", lt.Name, lt.Outward, lt.Inward))
	}
	if len(descriptions) == 0 {
		return issueLinkType{}, false, fmt.Errorf("this Jira has no issue link types configured")
	}
	return issueLinkType{}, false, fmt.Errorf("unknown link_type %q; this Jira has: %s",
		value, strings.Join(descriptions, ", "))
}

// listIssueLinkTypes returns the instance's issue link type catalog.
func (c *Client) listIssueLinkTypes() ([]issueLinkType, error) {
	raw, err := c.get("/rest/api/2/issueLinkType")
	if err != nil {
		return nil, err
	}
	var catalog struct {
		IssueLinkTypes []issueLinkType `json:"issueLinkTypes"`
	}
	if err := json.Unmarshal([]byte(raw), &catalog); err != nil {
		return nil, fmt.Errorf("cannot parse issue link type catalog: %w", err)
	}
	return catalog.IssueLinkTypes, nil
}

// GetIssueLinkTypes returns the issue link types configured on this Jira
// instance (their names and direction descriptions are what LinkIssues accepts).
func (c *Client) GetIssueLinkTypes() (string, error) {
	return c.get("/rest/api/2/issueLinkType")
}

// LinkIssues links two issues, reading as "key linkType targetKey" —
// LinkIssues("DEV-1", "blocks", "DEV-2", "") makes DEV-1 block DEV-2. linkType
// is a type name, a type id, or either direction's description; naming the
// inward direction reverses the ends. comment is an optional Jira wiki markup
// comment posted on key alongside the link.
func (c *Client) LinkIssues(key, linkType, targetKey, comment string) (string, error) {
	if key == "" || targetKey == "" {
		return "", fmt.Errorf("both issue_key and target_issue_key are required")
	}

	types, err := c.listIssueLinkTypes()
	if err != nil {
		return "", err
	}
	lt, swap, err := matchIssueLinkType(types, linkType)
	if err != nil {
		return "", err
	}

	// Jira's model is "outwardIssue <outward description> inwardIssue".
	outward, inward, description := key, targetKey, lt.Outward
	if swap {
		outward, inward, description = targetKey, key, lt.Inward
	}

	payload := map[string]any{
		"type":         map[string]any{"id": lt.ID},
		"outwardIssue": map[string]any{"key": outward},
		"inwardIssue":  map[string]any{"key": inward},
	}
	if comment != "" {
		payload["comment"] = map[string]any{"body": comment}
	}

	body, _ := json.Marshal(payload)
	if _, err := c.do(http.MethodPost, "/rest/api/2/issueLink", string(body)); err != nil {
		return "", err
	}

	return fmt.Sprintf("Linked %s %s %s", key, description, targetKey), nil
}

// DeleteIssueLink removes an issue link by its id (the ids are in an issue's
// "issuelinks" field, from GetIssue).
func (c *Client) DeleteIssueLink(linkID string) error {
	if linkID == "" {
		return fmt.Errorf("link_id is required")
	}
	_, err := c.do(http.MethodDelete, "/rest/api/2/issueLink/"+url.PathEscape(linkID), "")
	return err
}
