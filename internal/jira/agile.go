package jira

// Jira Software's Agile API. It lives under its own base path
// (/rest/agile/1.0) rather than the platform's /rest/api/2, and only exists
// when Jira Software is installed, so the failure modes here differ from the
// rest of this client.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// agileBase is the Jira Software REST base path.
	agileBase = "/rest/agile/1.0"

	// agileMoveLimit is the number of issues Jira accepts in a single move
	// request. A longer list is rejected outright, not truncated, so callers
	// must be chunked to this size.
	agileMoveLimit = 50

	// sprintTimeFormat is the ISO 8601 layout the Agile API uses for a sprint's
	// start and end dates. It differs from the worklog layout by the colon in
	// the zone offset.
	sprintTimeFormat = "2006-01-02T15:04:05.000-07:00"
)

// sprintDateLayouts are the inputs accepted for a sprint start or end date,
// most specific first. Layouts without a zone are read in the local time zone.
var sprintDateLayouts = []string{
	sprintTimeFormat,
	"2006-01-02T15:04:05.000-0700",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// sprintStates are the states GET .../sprint accepts.
var sprintStates = []string{"future", "active", "closed"}

// explainAgileError annotates a 404 from the Agile API. A bare 404 is by far
// the likeliest failure here and the hardest to interpret: it covers "this Jira
// has no Jira Software", "no such board/sprint/epic" and "you cannot see it"
// alike, and Jira's body says none of them.
func explainAgileError(err error) error {
	if err == nil || !strings.Contains(err.Error(), "jira error 404") {
		return err
	}
	return fmt.Errorf("the Jira Agile API (%s) answered 404: this instance may not have Jira Software installed, "+
		"or the board, sprint or epic does not exist or is not visible to your account (%w)", agileBase, err)
}

// splitAgileList splits a comma-separated argument into its trimmed, non-empty parts.
func splitAgileList(value string) []string {
	var parts []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// splitIssueKeys parses the comma-separated issue list every Agile move takes.
func splitIssueKeys(value string) ([]string, error) {
	keys := splitAgileList(value)
	if len(keys) == 0 {
		return nil, fmt.Errorf(`issue_keys is required, e.g. "DEV-1, DEV-2"`)
	}
	return keys, nil
}

// chunkIssues splits keys into groups of at most size, since Jira caps a move
// at agileMoveLimit issues per request.
func chunkIssues(keys []string, size int) [][]string {
	if size < 1 || len(keys) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(keys)+size-1)/size)
	for start := 0; start < len(keys); start += size {
		end := start + size
		if end > len(keys) {
			end = len(keys)
		}
		chunks = append(chunks, keys[start:end])
	}
	return chunks
}

// agileID validates a numeric Agile id. Jira answers a non-numeric id with the
// same opaque 404 it uses for "no such board", so reject it here instead.
func agileID(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s_id is required", kind)
	}
	if _, err := strconv.Atoi(value); err != nil {
		return "", fmt.Errorf("%s_id must be a numeric id, e.g. \"42\", got %q", kind, value)
	}
	return value, nil
}

// normalizeSprintState validates the comma-separated sprint state filter. The
// states are fixed by the API (unlike a workflow status), so an unknown one is
// a caller mistake worth naming.
func normalizeSprintState(value string) (string, error) {
	states := splitAgileList(strings.ToLower(value))
	if len(states) == 0 {
		return "", nil
	}
	for _, s := range states {
		valid := false
		for _, known := range sprintStates {
			if s == known {
				valid = true
				break
			}
		}
		if !valid {
			return "", fmt.Errorf("unknown sprint state %q; use %s (comma-separated)", s, strings.Join(sprintStates, ", "))
		}
	}
	return strings.Join(states, ","), nil
}

// normalizeSprintDate converts a sprint start or end date into the ISO 8601
// layout the Agile API expects. An empty value stays empty, leaving the sprint
// without that date.
func normalizeSprintDate(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, layout := range sprintDateLayouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t.Format(sprintTimeFormat), nil
		}
	}
	return "", fmt.Errorf("cannot parse %s %q; use yyyy-MM-dd, 'yyyy-MM-dd HH:mm' or %s", name, value, sprintTimeFormat)
}

// agileQuery builds the query string shared by the Agile list endpoints. jql
// and fields are omitted when empty, and fields is re-joined so a spaced list
// ("summary, status") reaches Jira as the bare field names it expects. startAt
// is the 0-based offset of the first result: the Agile API caps a page at 50,
// so it is the only way past the first 50 of a large board.
func agileQuery(jql, fields string, limit, startAt int) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("maxResults", strconv.Itoa(limit))
	}
	if startAt > 0 {
		q.Set("startAt", strconv.Itoa(startAt))
	}
	if jql = strings.TrimSpace(jql); jql != "" {
		q.Set("jql", jql)
	}
	if names := splitAgileList(fields); len(names) > 0 {
		q.Set("fields", strings.Join(names, ","))
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// agileGet performs a GET against the Agile API.
func (c *Client) agileGet(path string) (string, error) {
	raw, err := c.get(agileBase + path)
	if err != nil {
		return "", explainAgileError(err)
	}
	return raw, nil
}

// moveIssues posts an issue list to an Agile move endpoint, one chunk of
// agileMoveLimit at a time, and reports how many issues were moved. A failure
// part-way through still reports the issues already moved, because the earlier
// chunks are not rolled back.
func (c *Client) moveIssues(path string, keys []string) (int, error) {
	moved := 0
	for _, chunk := range chunkIssues(keys, agileMoveLimit) {
		body, _ := json.Marshal(map[string]any{"issues": chunk})
		if _, err := c.do(http.MethodPost, agileBase+path, string(body)); err != nil {
			return moved, fmt.Errorf("moved %d issue(s) before failing: %w", moved, explainAgileError(err))
		}
		moved += len(chunk)
	}
	return moved, nil
}

// GetBoards returns the Jira Software boards visible to the caller. projectKey
// and name are optional filters; name matches a substring of the board name.
// startAt pages past the first limit boards.
func (c *Client) GetBoards(projectKey, name string, limit, startAt int) (string, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("maxResults", strconv.Itoa(limit))
	}
	if startAt > 0 {
		q.Set("startAt", strconv.Itoa(startAt))
	}
	if projectKey = strings.TrimSpace(projectKey); projectKey != "" {
		q.Set("projectKeyOrId", projectKey)
	}
	if name = strings.TrimSpace(name); name != "" {
		q.Set("name", name)
	}

	path := "/board"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.agileGet(path)
}

// GetSprints returns a board's sprints. state is an optional comma-separated
// filter of future, active and closed, and startAt pages past the first limit
// sprints (a response with "isLast": false has more).
func (c *Client) GetSprints(boardID, state string, limit, startAt int) (string, error) {
	id, err := agileID("board", boardID)
	if err != nil {
		return "", err
	}
	states, err := normalizeSprintState(state)
	if err != nil {
		return "", err
	}

	q := url.Values{}
	if limit > 0 {
		q.Set("maxResults", strconv.Itoa(limit))
	}
	if startAt > 0 {
		q.Set("startAt", strconv.Itoa(startAt))
	}
	if states != "" {
		q.Set("state", states)
	}

	path := "/board/" + url.PathEscape(id) + "/sprint"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.agileGet(path)
}

// GetSprintIssues returns the issues in a sprint. jql narrows the set further
// and fields is a comma-separated list of fields to return, which keeps a big
// sprint's response manageable.
func (c *Client) GetSprintIssues(sprintID, jql, fields string, limit, startAt int) (string, error) {
	id, err := agileID("sprint", sprintID)
	if err != nil {
		return "", err
	}
	return c.agileGet("/sprint/" + url.PathEscape(id) + "/issue" + agileQuery(jql, fields, limit, startAt))
}

// GetBacklog returns a board's backlog, i.e. the issues on the board that are
// in no sprint. jql and fields narrow the result as in GetSprintIssues.
func (c *Client) GetBacklog(boardID, jql, fields string, limit, startAt int) (string, error) {
	id, err := agileID("board", boardID)
	if err != nil {
		return "", err
	}
	return c.agileGet("/board/" + url.PathEscape(id) + "/backlog" + agileQuery(jql, fields, limit, startAt))
}

// GetEpicIssues returns the issues belonging to an epic.
func (c *Client) GetEpicIssues(epicKey, fields string, limit, startAt int) (string, error) {
	epicKey = strings.TrimSpace(epicKey)
	if epicKey == "" {
		return "", fmt.Errorf("epic_key is required, e.g. \"DEV-42\"")
	}
	return c.agileGet("/epic/" + url.PathEscape(epicKey) + "/issue" + agileQuery("", fields, limit, startAt))
}

// CreateSprint creates a future sprint on a board. startDate and endDate are
// optional ISO 8601 timestamps and goal an optional sprint goal; a sprint is
// started later from the board, not here.
func (c *Client) CreateSprint(name, boardID, startDate, endDate, goal string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required, e.g. \"Sprint 12\"")
	}
	id, err := agileID("board", boardID)
	if err != nil {
		return "", err
	}
	originBoardID, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("board_id %q is out of range", id)
	}
	start, err := normalizeSprintDate("start_date", startDate)
	if err != nil {
		return "", err
	}
	end, err := normalizeSprintDate("end_date", endDate)
	if err != nil {
		return "", err
	}

	payload := map[string]any{"name": name, "originBoardId": originBoardID}
	if start != "" {
		payload["startDate"] = start
	}
	if end != "" {
		payload["endDate"] = end
	}
	if goal = strings.TrimSpace(goal); goal != "" {
		payload["goal"] = goal
	}

	body, _ := json.Marshal(payload)
	raw, err := c.do(http.MethodPost, agileBase+"/sprint", string(body))
	if err != nil {
		return "", explainAgileError(err)
	}
	return raw, nil
}

// MoveIssuesToSprint moves a comma-separated list of issues into a sprint. Jira
// only accepts future and active sprints as a destination.
func (c *Client) MoveIssuesToSprint(sprintID, issueKeys string) (string, error) {
	id, err := agileID("sprint", sprintID)
	if err != nil {
		return "", err
	}
	keys, err := splitIssueKeys(issueKeys)
	if err != nil {
		return "", err
	}

	moved, err := c.moveIssues("/sprint/"+url.PathEscape(id)+"/issue", keys)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Moved %d issue(s) to sprint %s", moved, id), nil
}

// MoveIssuesToBacklog removes a comma-separated list of issues from whatever
// sprint they are in, putting them back on their board's backlog.
func (c *Client) MoveIssuesToBacklog(issueKeys string) (string, error) {
	keys, err := splitIssueKeys(issueKeys)
	if err != nil {
		return "", err
	}

	moved, err := c.moveIssues("/backlog/issue", keys)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Moved %d issue(s) to the backlog", moved), nil
}

// MoveIssuesToEpic sets the epic of a comma-separated list of issues. The epic
// key "none" is the API's own convention for removing the issues from their
// current epic.
func (c *Client) MoveIssuesToEpic(epicKey, issueKeys string) (string, error) {
	epicKey = strings.TrimSpace(epicKey)
	if epicKey == "" {
		return "", fmt.Errorf("epic_key is required, e.g. \"DEV-42\" (or \"none\" to remove the issues from their epic)")
	}
	keys, err := splitIssueKeys(issueKeys)
	if err != nil {
		return "", err
	}

	moved, err := c.moveIssues("/epic/"+url.PathEscape(epicKey)+"/issue", keys)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(epicKey, "none") {
		return fmt.Sprintf("Removed %d issue(s) from their epic", moved), nil
	}
	return fmt.Sprintf("Moved %d issue(s) to epic %s", moved, epicKey), nil
}
