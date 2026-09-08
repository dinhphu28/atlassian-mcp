package jira

// Xray (Test Execution) support over the Xray Server/Data Center "Raven" REST
// API. Xray rides the same host and Personal Access Token as Jira, so these
// methods reuse the Jira Client's base URL, token, and HTTP helpers.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GetTestRun returns the Xray test run for a test within a test execution,
// including its id, status, steps, and defects.
func (c *Client) GetTestRun(testExecKey, testKey string) (string, error) {
	return c.get(fmt.Sprintf("/rest/raven/1.0/api/testrun?testExecIssueKey=%s&testIssueKey=%s",
		url.QueryEscape(testExecKey), url.QueryEscape(testKey)))
}

// ListTestStatuses returns the test statuses configured on the instance (PASS,
// FAIL, TODO, EXECUTING, ...), i.e. the values accepted by the status setters.
func (c *Client) ListTestStatuses() (string, error) {
	return c.get("/rest/raven/1.0/api/settings/teststatuses")
}

// ResolveTestRunID returns a test run's numeric id. runID is returned as-is when
// non-empty; otherwise it is looked up from the execution + test keys.
func (c *Client) ResolveTestRunID(runID, testExecKey, testKey string) (string, error) {
	if runID != "" {
		return runID, nil
	}
	if testExecKey == "" || testKey == "" {
		return "", fmt.Errorf("provide test_run_id, or both test_exec_key and test_issue_key")
	}
	raw, err := c.GetTestRun(testExecKey, testKey)
	if err != nil {
		return "", err
	}
	var r struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return "", fmt.Errorf("cannot parse test run: %w", err)
	}
	if r.ID.String() == "" {
		return "", fmt.Errorf("no test run found for test %s in execution %s", testKey, testExecKey)
	}
	return r.ID.String(), nil
}

// SetTestRunStatus sets a test run's status (e.g. PASS, FAIL, TODO).
func (c *Client) SetTestRunStatus(runID, status string) error {
	_, err := c.do(http.MethodPut,
		"/rest/raven/1.0/api/testrun/"+url.PathEscape(runID)+"/status?status="+url.QueryEscape(status), "")
	return err
}

// SetTestStepStatus sets the status of a single step within a test run.
func (c *Client) SetTestStepStatus(runID, stepID, status string) error {
	_, err := c.do(http.MethodPut,
		"/rest/raven/1.0/api/testrun/"+url.PathEscape(runID)+"/step/"+url.PathEscape(stepID)+
			"/status?status="+url.QueryEscape(status), "")
	return err
}

// SetTestRunComment sets a test run's comment. The Raven comment endpoint stores
// the raw request body as the comment text (a JSON content type is required even
// though the body is plain text); an empty comment clears it.
func (c *Client) SetTestRunComment(runID, comment string) error {
	req, err := http.NewRequest(http.MethodPut,
		c.baseURL+"/rest/raven/1.0/api/testrun/"+url.PathEscape(runID)+"/comment",
		strings.NewReader(comment))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("xray error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// UpdateExecutionTests adds and/or removes tests on a test execution.
func (c *Client) UpdateExecutionTests(execKey string, add, remove []string) (string, error) {
	payload := map[string]any{}
	if len(add) > 0 {
		payload["add"] = add
	}
	if len(remove) > 0 {
		payload["remove"] = remove
	}
	body, _ := json.Marshal(payload)
	return c.do(http.MethodPost,
		"/rest/raven/1.0/api/testexec/"+url.PathEscape(execKey)+"/test", string(body))
}

// UpdateTestRunDefects links and/or unlinks defect issues on a test run.
func (c *Client) UpdateTestRunDefects(runID string, add, remove []string) error {
	defects := map[string]any{}
	if len(add) > 0 {
		defects["add"] = add
	}
	if len(remove) > 0 {
		defects["remove"] = remove
	}
	body, _ := json.Marshal(map[string]any{"defects": defects})
	_, err := c.do(http.MethodPut,
		"/rest/raven/1.0/api/testrun/"+url.PathEscape(runID), string(body))
	return err
}
