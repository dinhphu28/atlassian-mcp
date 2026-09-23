package mcpserver

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

// xrayNote is appended to every tool description here: all of these paths are
// served by the Xray plugin, not Jira, and the Server/DC "Raven" API they use
// does not exist on Xray Cloud.
const xrayNote = " Requires the Xray for Jira Server/Data Center plugin (Raven REST API v1.0); not available on Xray Cloud."

func registerXrayTestReadTools(s *server.MCPServer, client *jira.Client) {
	getStepsTool := mcp.NewTool(
		"xray_get_test_steps",
		mcp.WithDescription("List the steps defined on an Xray Test issue (id, index, action, data, expected result). "+
			"These are the step definitions; for a step's result in a run use xray_get_test_run."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
	)

	s.AddTool(getStepsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestSteps(testKey))
	})

	listStepStatusesTool := mcp.NewTool(
		"xray_list_test_step_statuses",
		mcp.WithDescription("List the Xray test STEP statuses configured on this instance. "+
			"This is a separate catalog from xray_list_test_statuses, which covers whole test runs."+xrayNote),
	)

	s.AddTool(listStepStatusesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.ListTestStepStatuses())
	})

	getPreconditionsTool := mcp.NewTool(
		"xray_get_test_preconditions",
		mcp.WithDescription("List the Preconditions associated with an Xray Test issue."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
	)

	s.AddTool(getPreconditionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestPreconditions(testKey))
	})

	getPreconditionTestsTool := mcp.NewTool(
		"xray_get_precondition_tests",
		mcp.WithDescription("List the Tests associated with an Xray Precondition issue "+
			"(the reverse of xray_get_test_preconditions)."+xrayNote),
		mcp.WithString("precondition_key", mcp.Required(), mcp.Description("Precondition issue key (e.g. DEV-7)")),
		mcp.WithNumber("limit", mcp.Description("Maximum results per page (Xray's own default when omitted)")),
		mcp.WithNumber("page", mcp.Description("1-based page number (Xray's own default when omitted)")),
	)

	s.AddTool(getPreconditionTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("precondition_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetPreconditionTests(key, request.GetInt("limit", 0), request.GetInt("page", 0)))
	})

	getPlanTestsTool := mcp.NewTool(
		"xray_get_test_plan_tests",
		mcp.WithDescription("List the Tests on an Xray Test Plan, with each test's consolidated status."+xrayNote),
		mcp.WithString("test_plan_key", mcp.Required(), mcp.Description("Test Plan issue key (e.g. DEV-200)")),
		mcp.WithNumber("limit", mcp.Description("Maximum results per page (Xray's own default when omitted)")),
		mcp.WithNumber("page", mcp.Description("1-based page number (Xray's own default when omitted)")),
	)

	s.AddTool(getPlanTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		planKey, err := request.RequireString("test_plan_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestPlanTests(planKey, request.GetInt("limit", 0), request.GetInt("page", 0)))
	})

	getSetTestsTool := mcp.NewTool(
		"xray_get_test_set_tests",
		mcp.WithDescription("List the Tests on an Xray Test Set."+xrayNote),
		mcp.WithString("test_set_key", mcp.Required(), mcp.Description("Test Set issue key (e.g. DEV-300)")),
		mcp.WithNumber("limit", mcp.Description("Maximum results per page (Xray's own default when omitted)")),
		mcp.WithNumber("page", mcp.Description("1-based page number (Xray's own default when omitted)")),
	)

	s.AddTool(getSetTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		setKey, err := request.RequireString("test_set_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestSetTests(setKey, request.GetInt("limit", 0), request.GetInt("page", 0)))
	})

	getTestPlansTool := mcp.NewTool(
		"xray_get_test_plans_of_test",
		mcp.WithDescription("List the Xray Test Plans a Test issue belongs to."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
	)

	s.AddTool(getTestPlansTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestPlansOfTest(testKey))
	})

	getTestSetsTool := mcp.NewTool(
		"xray_get_test_sets_of_test",
		mcp.WithDescription("List the Xray Test Sets a Test issue belongs to."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
	)

	s.AddTool(getTestSetsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.GetTestSetsOfTest(testKey))
	})
}

func registerXrayTestWriteTools(s *server.MCPServer, client *jira.Client) {
	addStepTool := mcp.NewTool(
		"xray_add_test_step",
		mcp.WithDescription("Append a step to an Xray Test issue's definition. "+
			"This edits the Test itself, not a run; to record a step's result use xray_set_test_step_status."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
		mcp.WithString("action", mcp.Required(), mcp.Description("The step's action, e.g. \"Open the login page\"")),
		mcp.WithString("data", mcp.Description("The step's test data (optional)")),
		mcp.WithString("expected_result", mcp.Description("The step's expected result (optional)")),
	)

	s.AddTool(addStepTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		action, err := request.RequireString("action")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg, err := client.AddTestStep(
			testKey,
			action,
			request.GetString("data", ""),
			request.GetString("expected_result", ""),
		)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(msg), nil
	})

	updateStepTool := mcp.NewTool(
		"xray_update_test_step",
		mcp.WithDescription("Edit one step of an Xray Test issue's definition (get the step id from xray_get_test_steps). "+
			"Only the fields you supply are sent; the others keep their current value."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
		mcp.WithString("step_id", mcp.Required(), mcp.Description("Step id from xray_get_test_steps")),
		mcp.WithString("action", mcp.Description("New action text (unchanged if omitted)")),
		mcp.WithString("data", mcp.Description("New test data (unchanged if omitted)")),
		mcp.WithString("expected_result", mcp.Description("New expected result (unchanged if omitted)")),
	)

	s.AddTool(updateStepTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		stepID, err := request.RequireString("step_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg, err := client.UpdateTestStep(
			testKey,
			stepID,
			request.GetString("action", ""),
			request.GetString("data", ""),
			request.GetString("expected_result", ""),
		)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(msg), nil
	})

	deleteStepTool := mcp.NewTool(
		"xray_delete_test_step",
		mcp.WithDescription("Delete one step from an Xray Test issue's definition (get the step id from xray_get_test_steps)."+xrayNote),
		mcp.WithString("test_issue_key", mcp.Required(), mcp.Description("Test issue key (e.g. DEV-42)")),
		mcp.WithString("step_id", mcp.Required(), mcp.Description("Step id from xray_get_test_steps")),
	)

	s.AddTool(deleteStepTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		testKey, err := request.RequireString("test_issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		stepID, err := request.RequireString("step_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := client.DeleteTestStep(testKey, stepID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deleted step %s of %s", stepID, testKey)), nil
	})

	updatePlanTestsTool := mcp.NewTool(
		"xray_update_test_plan_tests",
		mcp.WithDescription("Add and/or remove Tests on an Xray Test Plan. "+
			"'add' also accepts Test Set keys, which expand to the Tests they contain; 'remove' takes Test keys only."+xrayNote),
		mcp.WithString("test_plan_key", mcp.Required(), mcp.Description("Test Plan issue key (e.g. DEV-200)")),
		mcp.WithString("add", mcp.Description("Comma-separated Test (or Test Set) keys to add, e.g. DEV-42,DEV-43")),
		mcp.WithString("remove", mcp.Description("Comma-separated Test keys to remove")),
	)

	s.AddTool(updatePlanTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		planKey, err := request.RequireString("test_plan_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.UpdateTestPlanTests(
			planKey,
			splitCSV(request.GetString("add", "")),
			splitCSV(request.GetString("remove", "")),
		))
	})

	updateSetTestsTool := mcp.NewTool(
		"xray_update_test_set_tests",
		mcp.WithDescription("Add and/or remove Tests on an Xray Test Set."+xrayNote),
		mcp.WithString("test_set_key", mcp.Required(), mcp.Description("Test Set issue key (e.g. DEV-300)")),
		mcp.WithString("add", mcp.Description("Comma-separated Test keys to add, e.g. DEV-42,DEV-43")),
		mcp.WithString("remove", mcp.Description("Comma-separated Test keys to remove")),
	)

	s.AddTool(updateSetTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		setKey, err := request.RequireString("test_set_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.UpdateTestSetTests(
			setKey,
			splitCSV(request.GetString("add", "")),
			splitCSV(request.GetString("remove", "")),
		))
	})

	updatePreconditionTestsTool := mcp.NewTool(
		"xray_update_precondition_tests",
		mcp.WithDescription("Associate and/or disassociate Tests on an Xray Precondition issue. "+
			"Xray only exposes this direction, so use the Precondition's key even when you are thinking about one Test."+xrayNote),
		mcp.WithString("precondition_key", mcp.Required(), mcp.Description("Precondition issue key (e.g. DEV-7)")),
		mcp.WithString("add", mcp.Description("Comma-separated Test keys to associate, e.g. DEV-42,DEV-43")),
		mcp.WithString("remove", mcp.Description("Comma-separated Test keys to disassociate")),
	)

	s.AddTool(updatePreconditionTestsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("precondition_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(client.UpdatePreconditionTests(
			key,
			splitCSV(request.GetString("add", "")),
			splitCSV(request.GetString("remove", "")),
		))
	})

	importResultsTool := mcp.NewTool(
		"xray_import_execution_results",
		mcp.WithDescription("Import test execution results in the Xray JSON format, creating a new Test Execution "+
			"or updating the one named by \"testExecutionKey\". Supply the payload inline as results_json or as a "+
			"local file_path. Only the Xray JSON format is supported here, not JUnit/Cucumber/TestNG reports."+xrayNote),
		mcp.WithString("results_json", mcp.Description(
			"Xray JSON payload, e.g. {\"info\":{\"summary\":\"nightly\"},\"tests\":[{\"testKey\":\"DEV-42\",\"status\":\"PASS\"}]}. "+
				"Use \"testExecutionKey\" instead of \"info\" to update an existing Test Execution.")),
		mcp.WithString("file_path", mcp.Description("Absolute path to a local file holding the Xray JSON payload; used when results_json is omitted")),
	)

	s.AddTool(importResultsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		results := request.GetString("results_json", "")
		if results == "" {
			filePath := request.GetString("file_path", "")
			if filePath == "" {
				return mcp.NewToolResultError("provide results_json or file_path"), nil
			}
			data, err := os.ReadFile(filePath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			results = string(data)
		}
		return jsonResult(client.ImportExecutionResults(results))
	})
}
