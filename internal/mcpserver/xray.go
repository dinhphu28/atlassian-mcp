package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

// splitCSV splits a comma-separated list into trimmed, non-empty items.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// runResolution reads the run-identifying params shared by the run-scoped tools:
// an explicit test_run_id, or an execution + test key pair to resolve it from.
func resolveRun(client *jira.Client, request mcp.CallToolRequest) (string, error) {
	return client.ResolveTestRunID(
		request.GetString("test_run_id", ""),
		request.GetString("test_exec_key", ""),
		request.GetString("test_issue_key", ""),
	)
}

// withRunParams adds the standard run-identifying parameters to a tool.
func withRunParams() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("test_run_id", mcp.Description("Test run id (from xray_get_test_run). If omitted, provide test_exec_key + test_issue_key instead")),
		mcp.WithString("test_exec_key", mcp.Description("Test Execution issue key (e.g. DEV-100); used with test_issue_key to locate the run")),
		mcp.WithString("test_issue_key", mcp.Description("Test issue key (e.g. DEV-42); used with test_exec_key to locate the run")),
	}
}

func registerXrayReadTools(s *server.MCPServer, client *jira.Client) {
	getTestRunTool := mcp.NewTool(
		"xray_get_test_run",
		mcp.WithDescription("Get an Xray test run (id, status, steps, defects) for a test within a test execution"),
		mcp.WithString("test_exec_key", mcp.Required(), mcp.Description("Test Execution issue key (e.g. DEV-100)")),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
	)

	s.AddTool(getTestRunTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		execKey, err := request.RequireString("test_exec_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestRun(execKey, testKey))
	})

	listStatusesTool := mcp.NewTool(
		"xray_list_test_statuses",
		mcp.WithDescription("List the Xray test statuses configured on this instance (the valid values for the status tools)"),
	)

	s.AddTool(listStatusesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.ListTestStatuses())
	})
}

func registerXrayWriteTools(s *server.MCPServer, client *jira.Client) {
	setRunStatusTool := mcp.NewTool(
		"xray_set_test_run_status",
		append([]mcp.ToolOption{
			mcp.WithDescription("Set an Xray test run's status (e.g. PASS, FAIL, TODO; see xray_list_test_statuses)"),
			mcp.WithString("status", mcp.Required(), mcp.Description("Status name, e.g. PASS, FAIL, TODO, EXECUTING")),
		}, withRunParams()...)...,
	)

	s.AddTool(setRunStatusTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status, err := request.RequireString("status")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		runID, err := resolveRun(client, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := client.SetTestRunStatus(runID, status); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Set test run %s status to %s", runID, status)), nil
	})

	setStepStatusTool := mcp.NewTool(
		"xray_set_test_step_status",
		append([]mcp.ToolOption{
			mcp.WithDescription("Set the status of a single step within an Xray test run (step ids from xray_get_test_run)"),
			mcp.WithString("step_id", mcp.Required(), mcp.Description("Step id from xray_get_test_run")),
			mcp.WithString("status", mcp.Required(), mcp.Description("Status name, e.g. PASS, FAIL, TODO")),
		}, withRunParams()...)...,
	)

	s.AddTool(setStepStatusTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stepID, err := request.RequireString("step_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		status, err := request.RequireString("status")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		runID, err := resolveRun(client, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := client.SetTestStepStatus(runID, stepID, status); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Set step %s of test run %s to %s", stepID, runID, status)), nil
	})

	setRunCommentTool := mcp.NewTool(
		"xray_set_test_run_comment",
		append([]mcp.ToolOption{
			mcp.WithDescription("Set (replace) an Xray test run's comment. An empty comment clears it."),
			mcp.WithString("comment", mcp.Description("Comment text; empty clears the comment")),
		}, withRunParams()...)...,
	)

	s.AddTool(setRunCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		runID, err := resolveRun(client, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := client.SetTestRunComment(runID, request.GetString("comment", "")); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Set comment on test run %s", runID)), nil
	})

	updateDefectsTool := mcp.NewTool(
		"xray_update_test_run_defects",
		append([]mcp.ToolOption{
			mcp.WithDescription("Link and/or unlink defect issues on an Xray test run"),
			mcp.WithString("add", mcp.Description("Comma-separated defect issue keys to link, e.g. DEV-1,DEV-2")),
			mcp.WithString("remove", mcp.Description("Comma-separated defect issue keys to unlink")),
		}, withRunParams()...)...,
	)

	s.AddTool(updateDefectsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		add := splitCSV(request.GetString("add", ""))
		remove := splitCSV(request.GetString("remove", ""))
		if len(add) == 0 && len(remove) == 0 {
			return mcp.NewToolResultError("provide 'add' and/or 'remove' defect keys"), nil
		}
		runID, err := resolveRun(client, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := client.UpdateTestRunDefects(runID, add, remove); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Updated defects on test run %s (add %v, remove %v)", runID, add, remove)), nil
	})

	updateExecTestsTool := mcp.NewTool(
		"xray_update_execution_tests",
		mcp.WithDescription("Add and/or remove tests on an Xray Test Execution"),
		mcp.WithString("test_exec_key", mcp.Required(), mcp.Description("Test Execution issue key (e.g. DEV-100)")),
		mcp.WithString("add", mcp.Description("Comma-separated Test issue keys to add, e.g. DEV-42,DEV-43")),
		mcp.WithString("remove", mcp.Description("Comma-separated Test issue keys to remove")),
	)

	s.AddTool(updateExecTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		execKey, err := request.RequireString("test_exec_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		add := splitCSV(request.GetString("add", ""))
		remove := splitCSV(request.GetString("remove", ""))
		if len(add) == 0 && len(remove) == 0 {
			return mcp.NewToolResultError("provide 'add' and/or 'remove' test keys"), nil
		}
		return jsonResult(client.UpdateExecutionTests(execKey, add, remove))
	})
}
