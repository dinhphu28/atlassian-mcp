package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func watchersPath(key string) string {
	return "/rest/api/2/issue/" + url.PathEscape(key) + "/watchers"
}

func votesPath(key string) string {
	return "/rest/api/2/issue/" + url.PathEscape(key) + "/votes"
}

// GetWatchers returns who is watching an issue.
func (c *Client) GetWatchers(key string) (string, error) {
	return c.get(watchersPath(key))
}

// AddWatcher adds a user to an issue's watcher list. username is a Server/DC
// username (not an email or a Cloud accountId).
func (c *Client) AddWatcher(key, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf(`username is required, e.g. "jsmith"`)
	}

	// This endpoint is the odd one out: the body is a bare JSON string, i.e.
	// "jsmith" and not {"name":"jsmith"}. Wrapping it in an object makes Jira
	// answer 400, so do not "fix" this into a struct.
	body, _ := json.Marshal(username)
	if _, err := c.do(http.MethodPost, watchersPath(key), string(body)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added %s as a watcher of %s", username, key), nil
}

// RemoveWatcher drops a user from an issue's watcher list. Unlike the add call,
// the username travels in the query string.
func (c *Client) RemoveWatcher(key, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf(`username is required, e.g. "jsmith"`)
	}

	q := url.Values{}
	q.Set("username", username)
	_, err := c.do(http.MethodDelete, watchersPath(key)+"?"+q.Encode(), "")
	return err
}

// GetVotes returns an issue's vote count and, when the instance exposes them,
// the voters.
func (c *Client) GetVotes(key string) (string, error) {
	return c.get(votesPath(key))
}

// Vote casts the token owner's vote on an issue. Jira refuses a vote on an
// issue you reported yourself, and that rejection is passed through as-is.
func (c *Client) Vote(key string) (string, error) {
	if _, err := c.do(http.MethodPost, votesPath(key), ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("Voted for %s", key), nil
}

// Unvote withdraws the token owner's vote from an issue.
func (c *Client) Unvote(key string) (string, error) {
	if _, err := c.do(http.MethodDelete, votesPath(key), ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed your vote from %s", key), nil
}
