package confluence

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// spaceKeyPattern is what Confluence Server/DC accepts for a space key: ASCII
// letters and digits only.
var spaceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// spaceEnumValue normalises one of the fixed enum parameters of the space
// endpoints (space type, space status, content type). An empty value stays
// empty, meaning "let Confluence decide"; anything else must be one of allowed,
// so a typo is rejected here naming the valid values rather than coming back as
// an opaque 400.
func spaceEnumValue(field, value string, allowed ...string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}

	for _, a := range allowed {
		if value == a {
			return value, nil
		}
	}

	return "", fmt.Errorf("unknown %s %q (use %s)", field, value, strings.Join(allowed, " or "))
}

// normalizeSpaceKey validates a key for a space about to be created and returns
// it uppercased, the convention every Confluence space key follows. Keys are
// matched case-insensitively by the API, so uppercasing changes nothing but the
// way the key reads afterwards.
func normalizeSpaceKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf(`key is required, e.g. "DEV"`)
	}
	if !spaceKeyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid space key %q; a key is letters and digits only - no spaces, "+
			"punctuation, underscores or accents - e.g. \"DEV\" or \"TEAM2\"", key)
	}
	if len(key) > 255 {
		return "", fmt.Errorf("space key %q is %d characters; Confluence allows at most 255", key, len(key))
	}

	return strings.ToUpper(key), nil
}

// spaceListQuery builds the query string for GET /rest/api/space.
func spaceListQuery(spaceType, status, label string, limit, start int) (string, error) {
	t, err := spaceEnumValue("space type", spaceType, "global", "personal")
	if err != nil {
		return "", err
	}
	st, err := spaceEnumValue("space status", status, "current", "archived")
	if err != nil {
		return "", err
	}

	q := url.Values{}
	if t != "" {
		q.Set("type", t)
	}
	if st != "" {
		q.Set("status", st)
	}
	if label = strings.TrimSpace(label); label != "" {
		q.Set("label", label)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if start > 0 {
		q.Set("start", strconv.Itoa(start))
	}

	if len(q) == 0 {
		return "", nil
	}
	return "?" + q.Encode(), nil
}

// spaceContentPath builds the path for a space's top-level content. contentType
// is optional; naming one restricts the listing to that single type instead of
// returning a page bucket and a blogpost bucket.
//
// depth is pinned to "root" because the endpoint defaults to "all", which
// returns content from every level of the tree: a caller asking for a space's
// roots would otherwise be handed arbitrary descendants and re-descend into
// them with GetDescendants.
func spaceContentPath(key, contentType string, limit, start int) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf(`space_key is required, e.g. "DEV"`)
	}
	ct, err := spaceEnumValue("content type", contentType, "page", "blogpost")
	if err != nil {
		return "", err
	}

	path := "/rest/api/space/" + url.PathEscape(key) + "/content"
	if ct != "" {
		path += "/" + ct
	}

	q := url.Values{}
	q.Set("depth", "root")
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if start > 0 {
		q.Set("start", strconv.Itoa(start))
	}
	path += "?" + q.Encode()

	return path, nil
}

// spaceSummary is one entry of a compacted space listing: what a caller needs to
// recognise a space and use its key.
type spaceSummary struct {
	Key  string      `json:"key"`
	Name string      `json:"name"`
	Type string      `json:"type"`
	ID   json.Number `json:"id,omitempty"`
}

// compactSpaceList reduces a GET /rest/api/space response to those fields. A
// full listing is mostly _links and _expandable per space, which crowds out the
// answer when all the caller wanted was to discover a space key. The paging
// envelope is kept so a truncated listing is still visibly truncated.
func compactSpaceList(raw string) (string, error) {
	var listing struct {
		Results []spaceSummary `json:"results"`
		Start   int            `json:"start"`
		Limit   int            `json:"limit"`
		Size    int            `json:"size"`
	}
	if err := json.Unmarshal([]byte(raw), &listing); err != nil {
		return "", fmt.Errorf("cannot parse space listing: %w", err)
	}
	if listing.Results == nil {
		listing.Results = []spaceSummary{}
	}

	compacted, err := json.Marshal(listing)
	if err != nil {
		return "", fmt.Errorf("cannot compact space listing: %w", err)
	}
	return string(compacted), nil
}

// userSearchCQL builds the CQL for a user search. Backslashes and quotes are
// escaped so a name carrying either cannot break out of the quoted term.
func userSearchCQL(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf(`query is required, e.g. "Jane"`)
	}

	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(query)
	return fmt.Sprintf(`user.fullname ~ "%s"`, escaped), nil
}

// GetSpaces lists the spaces on the instance. spaceType ("global" or
// "personal"), status ("current" or "archived") and label are all optional
// filters. compact reduces each entry to key, name, type and id.
func (c *Client) GetSpaces(spaceType, status, label string, limit, start int, compact bool) (string, error) {
	query, err := spaceListQuery(spaceType, status, label, limit, start)
	if err != nil {
		return "", err
	}

	raw, err := c.get("/rest/api/space" + query)
	if err != nil || !compact {
		return raw, err
	}
	return compactSpaceList(raw)
}

// GetSpace returns one space by key, expanded with its description, home page
// and labels. The homepage id is the root of the space's page tree.
func (c *Client) GetSpace(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf(`space_key is required, e.g. "DEV"`)
	}
	return c.get("/rest/api/space/" + url.PathEscape(key) + "?expand=description.plain,homepage,metadata.labels")
}

// GetSpaceContent returns the top-level content of a space. contentType is
// optional and narrows the listing to "page" or "blogpost".
func (c *Client) GetSpaceContent(key, contentType string, limit, start int) (string, error) {
	path, err := spaceContentPath(key, contentType, limit, start)
	if err != nil {
		return "", err
	}
	return c.get(path)
}

// GetCurrentUser returns the user the Personal Access Token belongs to.
func (c *Client) GetCurrentUser() (string, error) {
	return c.get("/rest/api/user/current")
}

// GetUser returns a user by username. Server/DC identifies users by username or
// user key, not by the accountId Cloud uses.
func (c *Client) GetUser(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	q := url.Values{}
	q.Set("username", username)
	return c.get("/rest/api/user?" + q.Encode())
}

// SearchUsers finds users whose full name matches query.
//
// Which endpoint serves user CQL is version-dependent: the dedicated
// /rest/api/search/user resource is the documented home of the user.* fields,
// while the general /rest/api/search dropped them at some point. Neither is
// guaranteed on a given Server/DC release, so the dedicated one is tried first
// and the general one is used as a fallback, mirroring how GetCreateMeta copes
// with the createmeta endpoint moving between versions. When both fail the
// first error is reported, since it names the endpoint that should have worked.
func (c *Client) SearchUsers(query string, limit int) (string, error) {
	cql, err := userSearchCQL(query)
	if err != nil {
		return "", err
	}

	q := url.Values{}
	q.Set("cql", cql)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	suffix := "?" + q.Encode()

	raw, err := c.get("/rest/api/search/user" + suffix)
	if err == nil {
		return raw, nil
	}
	if fallback, fallbackErr := c.get("/rest/api/search" + suffix); fallbackErr == nil {
		return fallback, nil
	}
	return "", fmt.Errorf("user search is not available on this Confluence "+
		"(tried /rest/api/search/user and /rest/api/search; look the user up by name with confluence_get_user): %w", err)
}

// CreateSpace creates a space. description is optional plain text. A private
// space is visible only to its creator until they grant access, and Confluence
// creates it through its own endpoint rather than a flag on the payload.
func (c *Client) CreateSpace(key, name, description string, private bool) (string, error) {
	key, err := normalizeSpaceKey(key)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf(`name is required, e.g. "Developer Docs"`)
	}

	payload := map[string]any{"key": key, "name": name}
	if description != "" {
		payload["description"] = map[string]any{
			"plain": map[string]any{"value": description, "representation": "plain"},
		}
	}

	path := "/rest/api/space"
	if private {
		path += "/_private"
	}

	body := marshalPayload(payload)
	return c.do(http.MethodPost, path, string(body))
}
