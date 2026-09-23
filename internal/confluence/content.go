package confluence

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// notFound reports whether err is a 404 from the REST API. Several endpoints
// below only graduated out of the /rest/experimental namespace in recent
// Confluence versions, so on an older Server/DC a 404 on the stable path means
// "not on this version here", not "no such content".
func notFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "confluence error 404")
}

// getFallback GETs suffix under /rest/api and retries it under
// /rest/experimental on a 404, so content restrictions and templates work on
// both the versions that expose them as stable and the older ones that still
// only have the experimental path.
func (c *Client) getFallback(suffix string) (string, error) {
	raw, err := c.get("/rest/api" + suffix)
	if !notFound(err) {
		return raw, err
	}
	return c.get("/rest/experimental" + suffix)
}

// doFallback is getFallback for a write.
func (c *Client) doFallback(method, suffix, body string) (string, error) {
	raw, err := c.do(method, "/rest/api"+suffix, body)
	if !notFound(err) {
		return raw, err
	}
	return c.do(method, "/rest/experimental"+suffix, body)
}

// contentSuffix is the /content/{id} part of a path, shared by the callers that
// have to prepend either API namespace.
func contentSuffix(pageID string) string {
	return "/content/" + url.PathEscape(pageID)
}

// DeleteLabel removes a label from a page. The name goes in the query string
// rather than the path because that is the form Server/DC documents; a label
// containing a slash would not survive a path segment.
func (c *Client) DeleteLabel(pageID, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("label name is required")
	}
	q := url.Values{}
	q.Set("name", name)
	_, err := c.do(http.MethodDelete,
		"/rest/api/content/"+url.PathEscape(pageID)+"/label?"+q.Encode(), "")
	return err
}

// GetDescendants lists every page in the subtree under a page, at any depth.
// No expand is requested: a subtree can be hundreds of pages, and id, title and
// links are all a caller navigating one needs.
func (c *Client) GetDescendants(pageID string, limit, start int) (string, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("start", strconv.Itoa(start))
	return c.get("/rest/api/content/" + url.PathEscape(pageID) + "/descendant/page?" + q.Encode())
}

// Breadcrumb is one step on the path from a space's root page down to a page.
type Breadcrumb struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// parseAncestors projects an expand=ancestors response to the breadcrumb trail.
// Confluence returns ancestors root-first, which is already the order a reader
// wants, and each raw entry carries a full content envelope that would dwarf
// the two fields that matter.
func parseAncestors(raw string) ([]Breadcrumb, error) {
	var page struct {
		Ancestors []Breadcrumb `json:"ancestors"`
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, fmt.Errorf("cannot parse ancestors: %w", err)
	}
	if page.Ancestors == nil {
		return []Breadcrumb{}, nil
	}
	return page.Ancestors, nil
}

// GetAncestors returns a page's ancestors, root first, as a JSON array of
// {id, title}. The page itself is not part of the trail.
func (c *Client) GetAncestors(pageID string) (string, error) {
	raw, err := c.get("/rest/api/content/" + url.PathEscape(pageID) + "?expand=ancestors")
	if err != nil {
		return "", err
	}

	crumbs, err := parseAncestors(raw)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(crumbs)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// restrictionUser identifies a restricted user. Server/DC keys users by
// username (and userKey); accountId is Cloud-only and never appears here.
type restrictionUser struct {
	Username string `json:"username"`
	UserKey  string `json:"userKey"`
}

// restrictionSet is who is allowed one operation on a page.
type restrictionSet struct {
	Users  []restrictionUser
	Groups []string
}

// restrictionInput is one operation's requested membership, as the tool layer
// receives it: a comma-separated list, the literal "none" to clear the
// operation, or empty to leave whatever the page already has.
type restrictionInput struct {
	Users  string
	Groups string
}

// keepUsers reports that the caller said nothing about this operation's users,
// so the page's current ones stand.
func (r restrictionInput) keepUsers() bool { return strings.TrimSpace(r.Users) == "" }

// keepGroups is keepUsers for groups.
func (r restrictionInput) keepGroups() bool { return strings.TrimSpace(r.Groups) == "" }

// splitList parses a comma-separated argument into its non-empty entries.
// "none" is the caller's way of saying "restrict nobody", so it yields an empty
// list rather than a member literally called none.
func splitList(value string) []string {
	if strings.EqualFold(strings.TrimSpace(value), "none") {
		return []string{}
	}

	out := []string{}
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseRestrictions reduces a byOperation response to who currently holds each
// operation, so an update that mentions only one operation can carry the other
// through unchanged.
func parseRestrictions(raw string) (map[string]restrictionSet, error) {
	var byOperation map[string]struct {
		Restrictions struct {
			User struct {
				Results []restrictionUser `json:"results"`
			} `json:"user"`
			Group struct {
				Results []struct {
					Name string `json:"name"`
				} `json:"results"`
			} `json:"group"`
		} `json:"restrictions"`
	}
	if err := json.Unmarshal([]byte(raw), &byOperation); err != nil {
		return nil, fmt.Errorf("cannot parse current restrictions: %w", err)
	}

	current := make(map[string]restrictionSet, len(byOperation))
	for operation, entry := range byOperation {
		set := restrictionSet{Users: []restrictionUser{}, Groups: []string{}}
		set.Users = append(set.Users, entry.Restrictions.User.Results...)
		for _, g := range entry.Restrictions.Group.Results {
			set.Groups = append(set.Groups, g.Name)
		}
		current[operation] = set
	}
	return current, nil
}

// userRef is the reference Confluence expects for a restricted user. The
// username is preferred, but a carried-over user read back from the server may
// only have a userKey, and dropping them would quietly widen access.
func userRef(u restrictionUser) map[string]any {
	if u.Username != "" {
		return map[string]any{"type": "known", "username": u.Username}
	}
	return map[string]any{"type": "known", "userKey": u.UserKey}
}

// restrictionEntry builds the payload element for one operation.
func restrictionEntry(operation string, set restrictionSet) map[string]any {
	users := make([]map[string]any, 0, len(set.Users))
	for _, u := range set.Users {
		users = append(users, userRef(u))
	}
	groups := make([]map[string]any, 0, len(set.Groups))
	for _, g := range set.Groups {
		groups = append(groups, map[string]any{"type": "group", "name": g})
	}

	return map[string]any{
		"operation": operation,
		"restrictions": map[string]any{
			"user":  users,
			"group": groups,
		},
	}
}

// resolveSet decides one operation's membership: the caller's list when they
// gave one, otherwise what the page already has.
func resolveSet(current restrictionSet, in restrictionInput) restrictionSet {
	out := restrictionSet{Users: current.Users, Groups: current.Groups}
	if out.Users == nil {
		out.Users = []restrictionUser{}
	}
	if out.Groups == nil {
		out.Groups = []string{}
	}

	if !in.keepUsers() {
		out.Users = []restrictionUser{}
		for _, name := range splitList(in.Users) {
			out.Users = append(out.Users, restrictionUser{Username: name})
		}
	}
	if !in.keepGroups() {
		out.Groups = splitList(in.Groups)
	}
	return out
}

// buildRestrictionPayload builds the whole PUT body. Both operations are always
// sent, because the PUT replaces the page's restrictions wholesale: an update
// that named only "read" and omitted "update" would otherwise unrestrict
// editing as a side effect. Operations the caller did not mention are re-sent
// exactly as they stand.
func buildRestrictionPayload(current map[string]restrictionSet, read, update restrictionInput) []map[string]any {
	return []map[string]any{
		restrictionEntry("read", resolveSet(current["read"], read)),
		restrictionEntry("update", resolveSet(current["update"], update)),
	}
}

// GetRestrictions returns who may read and who may update a page, grouped by
// operation.
func (c *Client) GetRestrictions(pageID string) (string, error) {
	return c.getFallback(contentSuffix(pageID) +
		"/restriction/byOperation?expand=restrictions.user,restrictions.group")
}

// SetRestrictions replaces a page's read and update restrictions. Each argument
// is a comma-separated list of usernames or group names, the literal "none" to
// restrict nobody for that operation, or empty to keep what the page already
// has. An empty restriction set means the operation is unrestricted, i.e.
// governed by space permissions alone.
func (c *Client) SetRestrictions(pageID, readUsers, readGroups, updateUsers, updateGroups string) (string, error) {
	raw, err := c.GetRestrictions(pageID)
	if err != nil {
		return "", err
	}
	current, err := parseRestrictions(raw)
	if err != nil {
		return "", err
	}

	payload := buildRestrictionPayload(current,
		restrictionInput{Users: readUsers, Groups: readGroups},
		restrictionInput{Users: updateUsers, Groups: updateGroups})

	body := marshalPayload(payload)
	return c.doFallback(http.MethodPut, contentSuffix(pageID)+"/restriction", string(body))
}

// propertyPath is the path of a single content property.
func propertyPath(pageID, key string) string {
	return "/rest/api/content/" + url.PathEscape(pageID) + "/property/" + url.PathEscape(key)
}

// GetProperties returns a page's content properties. An empty key lists them
// all; a key returns that one.
func (c *Client) GetProperties(pageID, key string) (string, error) {
	if key == "" {
		return c.get("/rest/api/content/" + url.PathEscape(pageID) + "/property?expand=value,version")
	}
	return c.get(propertyPath(pageID, key) + "?expand=value,version")
}

// SetProperty stores a JSON value under key on a page. value must be valid
// JSON: Confluence stores the property as structured data, not as text.
//
// Properties are versioned like pages, and the create and update calls differ,
// so the current version is read first: a missing property is POSTed, an
// existing one is PUT at version+1.
func (c *Client) SetProperty(pageID, key, value string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("property key is required")
	}

	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return "", fmt.Errorf("value must be JSON (object, array, string, number or boolean): %w", err)
	}

	raw, err := c.get(propertyPath(pageID, key) + "?expand=version")
	if notFound(err) {
		body := marshalPayload(map[string]any{"key": key, "value": parsed})
		return c.do(http.MethodPost,
			"/rest/api/content/"+url.PathEscape(pageID)+"/property", string(body))
	}
	if err != nil {
		return "", err
	}

	var current struct {
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot parse current property %s: %w", key, err)
	}

	body := marshalPayload(map[string]any{
		"key":     key,
		"value":   parsed,
		"version": map[string]any{"number": current.Version.Number + 1},
	})
	return c.do(http.MethodPut, propertyPath(pageID, key), string(body))
}

// DeleteProperty removes a content property from a page.
func (c *Client) DeleteProperty(pageID, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("property key is required")
	}
	_, err := c.do(http.MethodDelete, propertyPath(pageID, key), "")
	return err
}

// watchPath is the watch resource for the token's own user.
func watchPath(pageID string) string {
	return "/rest/api/user/watch/content/" + url.PathEscape(pageID)
}

// IsWatchingPage reports whether the token's own user watches a page.
func (c *Client) IsWatchingPage(pageID string) (string, error) {
	return c.get(watchPath(pageID))
}

// WatchPage starts or stops the token's own user watching a page.
//
// The POST carries an empty JSON object rather than no body at all: that sets a
// JSON content type, which keeps Confluence's XSRF filter from rejecting a
// bodyless POST it cannot tell apart from a forged form submission.
func (c *Client) WatchPage(pageID string, watch bool) (string, error) {
	if watch {
		if _, err := c.do(http.MethodPost, watchPath(pageID), "{}"); err != nil {
			return "", err
		}
		return fmt.Sprintf("Watching page %s", pageID), nil
	}

	if _, err := c.do(http.MethodDelete, watchPath(pageID), ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("Stopped watching page %s", pageID), nil
}

// GetTemplates lists the page templates of a space, or the global ones when
// spaceKey is empty. Bodies are not expanded; read one with GetTemplate.
func (c *Client) GetTemplates(spaceKey string, limit, start int) (string, error) {
	q := url.Values{}
	if spaceKey != "" {
		q.Set("spaceKey", spaceKey)
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("start", strconv.Itoa(start))
	return c.getFallback("/template/page?" + q.Encode())
}

// GetTemplate returns one template including its storage-format body.
func (c *Client) GetTemplate(templateID string) (string, error) {
	return c.getFallback("/template/" + url.PathEscape(templateID) + "?expand=body")
}

// copyTitle is the title of a copy: the caller's, or "Copy of <source>". Within
// one space a plain reuse of the source title would be rejected, since
// Confluence requires titles to be unique per space.
func copyTitle(newTitle, sourceTitle string) string {
	if t := strings.TrimSpace(newTitle); t != "" {
		return t
	}
	return "Copy of " + sourceTitle
}

// copySpace is the space a copy lands in: the caller's target, or the source's
// own space.
func copySpace(targetSpaceKey, sourceSpaceKey string) string {
	if s := strings.TrimSpace(targetSpaceKey); s != "" {
		return s
	}
	return sourceSpaceKey
}

// CopyPage creates a new page from a page's storage body, or a new blog post
// from a blog post's.
//
// The copy is done client-side rather than through POST /content/{id}/copy,
// which is a Cloud endpoint that Server/DC does not dependably expose
// (CONFSERVER-60397 is still open). Consequently only the body travels:
// attachments, child pages, labels, restrictions and comments do not, and any
// attachment reference inside the copied body dangles until the file is
// uploaded to the new page.
func (c *Client) CopyPage(pageID, targetSpaceKey, targetParentID, newTitle string) (string, error) {
	src, err := c.GetPageStorage(pageID)
	if err != nil {
		return "", err
	}

	// The copy keeps the source's own type: hardcoding "page" would quietly
	// turn a blog post into a page that never shows up in the space's blog feed.
	sourceType := contentTypeOf(src.Type)
	if sourceType == ContentTypeBlogpost && strings.TrimSpace(targetParentID) != "" {
		return "", fmt.Errorf("content %s is a %s, which has no parent page; omit target_parent_id",
			pageID, ContentTypeBlogpost)
	}

	return c.CreatePage(
		copySpace(targetSpaceKey, src.Space),
		copyTitle(newTitle, src.Title),
		src.Storage,
		targetParentID,
		"storage",
		sourceType,
	)
}
