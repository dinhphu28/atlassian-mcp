package jira

// Xray Test-definition support over the Xray Server/Data Center "Raven" REST
// API v1.0, covering the Test issue itself (its steps and preconditions), Test
// Plan and Test Set membership, and importing execution results. xray.go covers
// the other half, test runs. Every path here exists only when the Xray plugin
// is installed; Xray Cloud uses a different API and none of this applies there.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// xrayCleanKeys drops blank entries and surrounding whitespace from a list of
// issue keys, so a trailing comma in the caller's list is not sent as "".
func xrayCleanKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if t := strings.TrimSpace(k); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// xrayMembershipPayload builds the body shared by the Raven association
// endpoints (Test Plan, Test Set, Precondition). A list that is empty is left
// out of the body entirely rather than sent as [], and asking for neither is
// rejected here: Xray answers such a request with 200 and does nothing, which
// looks like success to the caller.
func xrayMembershipPayload(add, remove []string) (string, error) {
	added := xrayCleanKeys(add)
	removed := xrayCleanKeys(remove)
	if len(added) == 0 && len(removed) == 0 {
		return "", fmt.Errorf("provide 'add' and/or 'remove' issue keys")
	}

	payload := map[string]any{}
	if len(added) > 0 {
		payload["add"] = added
	}
	if len(removed) > 0 {
		payload["remove"] = removed
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// xrayPageQuery builds the optional limit/page query the Raven membership
// listings accept. Zero means "unset", leaving Xray's own default in force.
func xrayPageQuery(limit, page int) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// xrayTestStepPayload builds the body for a test step create or update. Xray
// names the three fields "step" (the action), "data" and "result" (the expected
// result). create requires an action, since a step without one is not usable in
// the Test's editor; an update sends only the fields the caller supplied, so
// the rest keep their current values.
func xrayTestStepPayload(action, data, expected string, create bool) (string, error) {
	action = strings.TrimSpace(action)

	payload := map[string]any{}
	if create {
		if action == "" {
			return "", fmt.Errorf(`action is required, e.g. "Open the login page"`)
		}
		payload["step"] = action
		payload["data"] = data
		payload["result"] = expected
	} else {
		if action == "" && data == "" && expected == "" {
			return "", fmt.Errorf("provide action, data and/or expected_result to update")
		}
		if action != "" {
			payload["step"] = action
		}
		if data != "" {
			payload["data"] = data
		}
		if expected != "" {
			payload["result"] = expected
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// xrayValidateExecutionImport rejects a payload that is not the Xray JSON
// execution-results format before it is sent. The import endpoint answers a
// malformed or mis-shaped body with a bare 500 and no explanation, so the
// cheap structural checks are worth doing here.
func xrayValidateExecutionImport(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("results are empty; provide results_json or file_path")
	}

	var payload struct {
		TestExecutionKey string            `json:"testExecutionKey"`
		Info             *json.RawMessage  `json:"info"`
		Tests            []json.RawMessage `json:"tests"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("results are not valid Xray JSON: %w", err)
	}

	if len(payload.Tests) == 0 {
		return fmt.Errorf(`results need a non-empty "tests" array, e.g. {"tests":[{"testKey":"DEV-42","status":"PASS"}]}`)
	}
	if payload.TestExecutionKey == "" && payload.Info == nil {
		return fmt.Errorf(`results need "testExecutionKey" to update an existing Test Execution, or "info" to create one`)
	}
	return nil
}

func xrayTestPath(testKey string) string {
	return "/rest/raven/1.0/api/test/" + url.PathEscape(testKey)
}

// GetTestSteps returns the steps defined on a Test issue, each with its id,
// index, action ("step"), data and expected result.
func (c *Client) GetTestSteps(testKey string) (string, error) {
	return c.get(xrayTestPath(testKey) + "/step")
}

// ListTestStepStatuses returns the step statuses configured on the instance.
// These are a separate catalog from the test statuses in ListTestStatuses.
func (c *Client) ListTestStepStatuses() (string, error) {
	return c.get("/rest/raven/1.0/api/settings/teststepstatuses")
}

// AddTestStep appends a step to a Test issue. Xray creates the step at the end
// of the list; the endpoint answers with an empty body, so this returns a
// confirmation instead.
func (c *Client) AddTestStep(testKey, action, data, expected string) (string, error) {
	body, err := xrayTestStepPayload(action, data, expected, true)
	if err != nil {
		return "", err
	}
	// Xray inverts the usual verbs here: PUT creates a step, POST edits one.
	if _, err := c.do(http.MethodPut, xrayTestPath(testKey)+"/step", body); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added a step to %s", testKey), nil
}

// UpdateTestStep edits one step of a Test issue. stepID comes from
// GetTestSteps; fields left empty keep their current value.
func (c *Client) UpdateTestStep(testKey, stepID, action, data, expected string) (string, error) {
	body, err := xrayTestStepPayload(action, data, expected, false)
	if err != nil {
		return "", err
	}
	if _, err := c.do(http.MethodPost,
		xrayTestPath(testKey)+"/step/"+url.PathEscape(stepID), body); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated step %s of %s", stepID, testKey), nil
}

// DeleteTestStep removes one step from a Test issue.
func (c *Client) DeleteTestStep(testKey, stepID string) error {
	_, err := c.do(http.MethodDelete,
		xrayTestPath(testKey)+"/step/"+url.PathEscape(stepID), "")
	return err
}

// GetTestPreconditions returns the Preconditions associated with a Test.
func (c *Client) GetTestPreconditions(testKey string) (string, error) {
	return c.get(xrayTestPath(testKey) + "/preconditions")
}

// GetTestSetsOfTest returns the Test Sets a Test belongs to.
func (c *Client) GetTestSetsOfTest(testKey string) (string, error) {
	return c.get(xrayTestPath(testKey) + "/testsets")
}

// GetTestPlansOfTest returns the Test Plans a Test belongs to.
func (c *Client) GetTestPlansOfTest(testKey string) (string, error) {
	return c.get(xrayTestPath(testKey) + "/testplans")
}

// GetPreconditionTests returns the Tests associated with a Precondition issue.
func (c *Client) GetPreconditionTests(preconditionKey string, limit, page int) (string, error) {
	return c.get("/rest/raven/1.0/api/precondition/" + url.PathEscape(preconditionKey) +
		"/test" + xrayPageQuery(limit, page))
}

// UpdatePreconditionTests associates and/or disassociates Tests on a
// Precondition. Xray only exposes this direction: there is no endpoint that
// attaches Preconditions to a Test.
func (c *Client) UpdatePreconditionTests(preconditionKey string, add, remove []string) (string, error) {
	body, err := xrayMembershipPayload(add, remove)
	if err != nil {
		return "", err
	}
	return c.do(http.MethodPost,
		"/rest/raven/1.0/api/precondition/"+url.PathEscape(preconditionKey)+"/test", body)
}

// GetTestPlanTests returns the Tests on a Test Plan with their consolidated
// status.
func (c *Client) GetTestPlanTests(planKey string, limit, page int) (string, error) {
	return c.get("/rest/raven/1.0/api/testplan/" + url.PathEscape(planKey) +
		"/test" + xrayPageQuery(limit, page))
}

// UpdateTestPlanTests adds and/or removes Tests on a Test Plan. "add" also
// accepts Test Set keys, which expand to the Tests they contain; "remove" takes
// Test keys only.
func (c *Client) UpdateTestPlanTests(planKey string, add, remove []string) (string, error) {
	body, err := xrayMembershipPayload(add, remove)
	if err != nil {
		return "", err
	}
	return c.do(http.MethodPost,
		"/rest/raven/1.0/api/testplan/"+url.PathEscape(planKey)+"/test", body)
}

// GetTestSetTests returns the Tests on a Test Set.
func (c *Client) GetTestSetTests(setKey string, limit, page int) (string, error) {
	return c.get("/rest/raven/1.0/api/testset/" + url.PathEscape(setKey) +
		"/test" + xrayPageQuery(limit, page))
}

// UpdateTestSetTests adds and/or removes Tests on a Test Set.
func (c *Client) UpdateTestSetTests(setKey string, add, remove []string) (string, error) {
	body, err := xrayMembershipPayload(add, remove)
	if err != nil {
		return "", err
	}
	return c.do(http.MethodPost,
		"/rest/raven/1.0/api/testset/"+url.PathEscape(setKey)+"/test", body)
}

// ImportExecutionResults posts results in the Xray JSON format, creating a new
// Test Execution or updating the one named by "testExecutionKey". It returns
// Xray's response, which identifies the affected Test Execution issue.
func (c *Client) ImportExecutionResults(results string) (string, error) {
	if err := xrayValidateExecutionImport(results); err != nil {
		return "", err
	}
	return c.do(http.MethodPost, "/rest/raven/1.0/import/execution", results)
}
