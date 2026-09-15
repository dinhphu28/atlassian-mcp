package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// jiraTimeFormat is the timestamp layout Jira requires for a worklog's
// "started" field. Jira rejects anything else, including plain RFC 3339.
const jiraTimeFormat = "2006-01-02T15:04:05.000-0700"

// startedLayouts are the inputs accepted for a worklog start time, most
// specific first. Layouts without a zone are read in the local time zone.
var startedLayouts = []string{
	jiraTimeFormat,
	"2006-01-02T15:04:05-0700",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// normalizeStarted converts a start time into the layout Jira expects. An empty
// value stays empty, which makes Jira default to "now".
func normalizeStarted(started string) (string, error) {
	started = strings.TrimSpace(started)
	if started == "" {
		return "", nil
	}

	for _, layout := range startedLayouts {
		if t, err := time.ParseInLocation(layout, started, time.Local); err == nil {
			return t.Format(jiraTimeFormat), nil
		}
	}

	return "", fmt.Errorf("cannot parse started %q; use yyyy-MM-dd, 'yyyy-MM-dd HH:mm' or %s", started, jiraTimeFormat)
}

// worklogQuery builds the adjustEstimate query string for a worklog write.
// manualParam names the parameter that carries value when adjust is "manual"
// ("reduceBy" when logging work, "increaseBy" when deleting it); an empty
// manualParam means the endpoint does not support "manual".
func worklogQuery(adjust, value, manualParam string) (string, error) {
	adjust = strings.ToLower(strings.TrimSpace(adjust))
	if adjust == "" {
		return "", nil
	}

	q := url.Values{}
	switch adjust {
	case "auto", "leave":
		q.Set("adjustEstimate", adjust)
	case "new":
		if value == "" {
			return "", fmt.Errorf(`adjust_estimate "new" requires estimate_value, e.g. "2d 4h"`)
		}
		q.Set("adjustEstimate", "new")
		q.Set("newEstimate", value)
	case "manual":
		if manualParam == "" {
			return "", fmt.Errorf(`adjust_estimate "manual" is not supported when updating a worklog`)
		}
		if value == "" {
			return "", fmt.Errorf(`adjust_estimate "manual" requires estimate_value, e.g. "2d 4h"`)
		}
		q.Set("adjustEstimate", "manual")
		q.Set(manualParam, value)
	default:
		return "", fmt.Errorf("unknown adjust_estimate %q (use auto, new, leave or manual)", adjust)
	}

	return "?" + q.Encode(), nil
}

func worklogPath(key string) string {
	return "/rest/api/2/issue/" + url.PathEscape(key) + "/worklog"
}

// GetWorklogs returns the work logged on an issue.
func (c *Client) GetWorklogs(key string) (string, error) {
	return c.get(worklogPath(key))
}

// AddWorklog logs work on an issue. timeSpent is a Jira duration such as
// "3h 30m"; started is optional (Jira defaults to now) and comment uses Jira
// wiki markup. adjustEstimate controls the remaining estimate and defaults to
// Jira's "auto"; estimateValue supplies newEstimate for "new" and reduceBy for
// "manual".
func (c *Client) AddWorklog(key, timeSpent, started, comment, adjustEstimate, estimateValue string) (string, error) {
	if timeSpent == "" {
		return "", fmt.Errorf(`time_spent is required, e.g. "3h 30m"`)
	}

	startedAt, err := normalizeStarted(started)
	if err != nil {
		return "", err
	}
	query, err := worklogQuery(adjustEstimate, estimateValue, "reduceBy")
	if err != nil {
		return "", err
	}

	payload := map[string]any{"timeSpent": timeSpent}
	if startedAt != "" {
		payload["started"] = startedAt
	}
	if comment != "" {
		payload["comment"] = comment
	}

	body, _ := json.Marshal(payload)
	return c.do(http.MethodPost, worklogPath(key)+query, string(body))
}

// UpdateWorklog edits an existing worklog. At least one of timeSpent, started
// or comment must be given; the others keep their current values. "manual" is
// not a valid adjustEstimate here, and an existing comment cannot be cleared.
func (c *Client) UpdateWorklog(key, worklogID, timeSpent, started, comment, adjustEstimate, estimateValue string) (string, error) {
	if timeSpent == "" && started == "" && comment == "" {
		return "", fmt.Errorf("provide time_spent, started and/or comment to update")
	}

	startedAt, err := normalizeStarted(started)
	if err != nil {
		return "", err
	}
	query, err := worklogQuery(adjustEstimate, estimateValue, "")
	if err != nil {
		return "", err
	}

	path := worklogPath(key) + "/" + url.PathEscape(worklogID)

	// Jira replaces the worklog with the body it is sent, so read the current
	// one and carry over whatever the caller did not override.
	raw, err := c.get(path)
	if err != nil {
		return "", err
	}
	var current struct {
		TimeSpentSeconds int    `json:"timeSpentSeconds"`
		Started          string `json:"started"`
		Comment          string `json:"comment"`
	}
	if err := json.Unmarshal([]byte(raw), &current); err != nil {
		return "", fmt.Errorf("cannot read worklog %s on %s: %w", worklogID, key, err)
	}

	payload := map[string]any{}
	if timeSpent != "" {
		payload["timeSpent"] = timeSpent
	} else {
		payload["timeSpentSeconds"] = current.TimeSpentSeconds
	}
	if startedAt != "" {
		payload["started"] = startedAt
	} else if current.Started != "" {
		payload["started"] = current.Started
	}
	if comment != "" {
		payload["comment"] = comment
	} else if current.Comment != "" {
		payload["comment"] = current.Comment
	}

	body, _ := json.Marshal(payload)
	return c.do(http.MethodPut, path+query, string(body))
}

// DeleteWorklog removes a worklog from an issue. adjustEstimate defaults to
// Jira's "auto"; estimateValue supplies newEstimate for "new" and increaseBy
// for "manual".
func (c *Client) DeleteWorklog(key, worklogID, adjustEstimate, estimateValue string) error {
	query, err := worklogQuery(adjustEstimate, estimateValue, "increaseBy")
	if err != nil {
		return err
	}

	_, err = c.do(http.MethodDelete, worklogPath(key)+"/"+url.PathEscape(worklogID)+query, "")
	return err
}
