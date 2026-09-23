package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func remoteLinkPath(key string) string {
	return "/rest/api/2/issue/" + url.PathEscape(key) + "/remotelink"
}

// remoteLinkPayload builds the body of a remote issue link write.
//
// Jira treats globalId as the identity of the link and upserts on it: posting
// the same globalId to the same issue twice updates the existing link rather
// than adding a duplicate. A caller linking a Confluence page rarely has a
// meaningful id of its own, so an empty globalID defaults to the link URL,
// which makes re-linking the same page idempotent for free.
//
// That upsert is a full replace: "fields not present in the request are
// interpreted as having a null value", so a re-link that omits summary,
// relationship, icon, status or application would wipe them off the existing
// link. existing is the link Jira currently holds for this globalId (nil when
// there is none); its optional fields are carried over and only what the caller
// supplied is overwritten.
func remoteLinkPayload(linkURL, title, summary, relationship, globalID, iconURL, iconTitle string, existing map[string]any) (map[string]any, error) {
	linkURL = strings.TrimSpace(linkURL)
	title = strings.TrimSpace(title)
	if linkURL == "" {
		return nil, fmt.Errorf("url is required, e.g. https://confluence.example.com/display/DOC/Page")
	}
	if title == "" {
		return nil, fmt.Errorf("title is required; it is the text shown on the issue")
	}

	payload := map[string]any{}
	object := map[string]any{}

	// Carry over whatever the link already has, so the caller only has to
	// resupply what they want changed.
	if existing != nil {
		for _, k := range []string{"relationship", "application"} {
			if v, ok := existing[k]; ok && v != nil {
				payload[k] = v
			}
		}
		if prev, ok := existing["object"].(map[string]any); ok {
			for _, k := range []string{"summary", "icon", "status"} {
				if v, ok := prev[k]; ok && v != nil {
					object[k] = v
				}
			}
		}
	}

	object["url"] = linkURL
	object["title"] = title
	if summary = strings.TrimSpace(summary); summary != "" {
		object["summary"] = summary
	}
	if iconURL = strings.TrimSpace(iconURL); iconURL != "" {
		icon := map[string]any{"url16x16": iconURL}
		if iconTitle = strings.TrimSpace(iconTitle); iconTitle != "" {
			icon["title"] = iconTitle
		}
		object["icon"] = icon
	}

	if globalID = strings.TrimSpace(globalID); globalID == "" {
		globalID = linkURL
	}

	payload["globalId"] = globalID
	payload["object"] = object
	if relationship = strings.TrimSpace(relationship); relationship != "" {
		payload["relationship"] = relationship
	}

	return payload, nil
}

// existingRemoteLink returns the link already stored under globalID on an
// issue, or nil when there is none. Jira answers a globalId lookup with either
// the bare link or a one-element array depending on version, and with a 404
// when nothing matches, so every failure here is treated as "no existing link"
// rather than aborting the write.
func (c *Client) existingRemoteLink(key, globalID string) map[string]any {
	q := url.Values{}
	q.Set("globalId", globalID)

	raw, err := c.get(remoteLinkPath(key) + "?" + q.Encode())
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}

	var single map[string]any
	if err := json.Unmarshal([]byte(raw), &single); err == nil {
		if _, ok := single["object"]; ok {
			return single
		}
		return nil
	}

	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err != nil || len(list) == 0 {
		return nil
	}
	return list[0]
}

// GetRemoteLinks returns an issue's remote links: its links out to Confluence
// pages, web pages and other applications.
func (c *Client) GetRemoteLinks(key string) (string, error) {
	return c.get(remoteLinkPath(key))
}

// AddRemoteLink links an issue out to a URL, typically a Confluence page.
// globalID is optional and defaults to linkURL, which makes repeat calls for
// the same target update the existing link instead of duplicating it.
//
// Because that update replaces the whole link, the current one is read first
// and its optional fields (summary, relationship, icon, status and the
// application it belongs to) are carried into the write unless the caller
// supplied a new value.
func (c *Client) AddRemoteLink(key, linkURL, title, summary, relationship, globalID, iconURL, iconTitle string) (string, error) {
	lookupID := strings.TrimSpace(globalID)
	if lookupID == "" {
		lookupID = strings.TrimSpace(linkURL)
	}

	var existing map[string]any
	if lookupID != "" {
		existing = c.existingRemoteLink(key, lookupID)
	}

	payload, err := remoteLinkPayload(linkURL, title, summary, relationship, globalID, iconURL, iconTitle, existing)
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(payload)
	return c.do(http.MethodPost, remoteLinkPath(key), string(body))
}

// remoteLinkDeleteQuery picks between the two ways Jira can address a remote
// link for deletion: by its own id (a path segment) or by the globalId it was
// created with (a query parameter). Exactly one is needed, and linkID wins when
// both are given because it is the more specific of the two.
func remoteLinkDeleteQuery(linkID, globalID string) (suffix string, err error) {
	linkID = strings.TrimSpace(linkID)
	globalID = strings.TrimSpace(globalID)

	switch {
	case linkID != "":
		return "/" + url.PathEscape(linkID), nil
	case globalID != "":
		// A globalId is routinely a URL or a "system=…&id=…" pair, so it has to
		// be escaped rather than concatenated.
		q := url.Values{}
		q.Set("globalId", globalID)
		return "?" + q.Encode(), nil
	default:
		return "", fmt.Errorf("provide link_id (from jira_get_remote_links) or global_id")
	}
}

// DeleteRemoteLink removes a remote link from an issue, addressed either by
// link id or by the globalId it was created with.
func (c *Client) DeleteRemoteLink(key, linkID, globalID string) error {
	suffix, err := remoteLinkDeleteQuery(linkID, globalID)
	if err != nil {
		return err
	}
	_, err = c.do(http.MethodDelete, remoteLinkPath(key)+suffix, "")
	return err
}
