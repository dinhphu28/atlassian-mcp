package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraMetaReadTools(s *server.MCPServer, client *jira.Client) {
	searchUsersTool := mcp.NewTool(
		"jira_search_users",
		mcp.WithDescription("Find Jira users. This is how you get the username that jira_assign_issue, "+
			"the assignee argument of jira_create_issue and jira_add_watcher need: it is the \"name\" of a result "+
			"(this is Jira Server/Data Center, which identifies users by username, not by accountId). "+
			"The query matches username, display name and email address."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Part of a username, display name or email address, e.g. 'alice'")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of users (default 20)")),
	)

	s.AddTool(searchUsersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.SearchUsers(query, request.GetInt("limit", 20)))
	})

	getMyselfTool := mcp.NewTool(
		"jira_get_myself",
		mcp.WithDescription("Get the Jira user the configured token belongs to: who this server acts as, "+
			"including the username to use in JQL such as 'assignee = currentUser()'"),
	)

	s.AddTool(getMyselfTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetMyself())
	})

	getProjectsTool := mcp.NewTool(
		"jira_get_projects",
		mcp.WithDescription("List the Jira projects visible to this token (their keys are what jira_create_issue "+
			"and JQL take). Compact by default, since the raw payload carries avatar URLs per project."),
		mcp.WithBoolean("compact", mcp.Description("Reduce each project to key, name, id and projectTypeKey (default true)")),
	)

	s.AddTool(getProjectsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetProjects(request.GetBool("compact", true)))
	})

	getCreateMetaTool := mcp.NewTool(
		"jira_get_create_meta",
		mcp.WithDescription("Discover what jira_create_issue must supply for a project: the issue types it accepts "+
			"and, per type, the fields its create screen requires with their allowed values. "+
			"Returns a compact projection; set raw=true for Jira's full createmeta payload. "+
			"Jira 9 removed the single createmeta resource, so on those instances this falls back to the "+
			"per-issue-type endpoints, which serve fields one issue type at a time - pass issue_type to get them."),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV (from jira_get_projects)")),
		mcp.WithString("issue_type", mcp.Description("Narrow the answer to one issue type name (e.g. Bug) or type id")),
		mcp.WithBoolean("raw", mcp.Description("Return Jira's untouched createmeta payload instead of the compact projection (default false)")),
	)

	s.AddTool(getCreateMetaTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectKey, err := request.RequireString("project_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetCreateMeta(
			projectKey,
			request.GetString("issue_type", ""),
			request.GetBool("raw", false),
		))
	})

	getProjectVersionsTool := mcp.NewTool(
		"jira_get_project_versions",
		mcp.WithDescription("List a project's versions/releases (their names and ids are what jira_set_fields "+
			"accepts for fix_versions and affects_versions)"),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV (from jira_get_projects)")),
	)

	s.AddTool(getProjectVersionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectKey, err := request.RequireString("project_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetProjectVersions(projectKey))
	})

	getProjectComponentsTool := mcp.NewTool(
		"jira_get_project_components",
		mcp.WithDescription("List a project's components (their names and ids are what jira_set_fields accepts for components)"),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV (from jira_get_projects)")),
	)

	s.AddTool(getProjectComponentsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectKey, err := request.RequireString("project_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetProjectComponents(projectKey))
	})

	getFiltersTool := mcp.NewTool(
		"jira_get_filters",
		mcp.WithDescription("List the caller's favourite saved filters, or get one filter by id. "+
			"A filter's \"jql\" can then be run with jira_search."),
		mcp.WithString("filter_id", mcp.Description("Filter id to fetch on its own (from a favourite filter); omit to list the favourites")),
	)

	s.AddTool(getFiltersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetFilters(request.GetString("filter_id", "")))
	})
}

func registerJiraMetaWriteTools(s *server.MCPServer, client *jira.Client) {
	createVersionTool := mcp.NewTool(
		"jira_create_version",
		mcp.WithDescription("Create a version (release) in a Jira project, so it can be used as a fix version. "+
			"List the existing ones with jira_get_project_versions."),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key, e.g. DEV (from jira_get_projects)")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Version name, e.g. 1.4.0")),
		mcp.WithString("description", mcp.Description("Version description")),
		mcp.WithString("start_date", mcp.Description("Start date in yyyy-MM-dd format")),
		mcp.WithString("release_date", mcp.Description("Release date in yyyy-MM-dd format")),
		mcp.WithBoolean("released", mcp.Description("Mark the version as already released (default false)")),
	)

	s.AddTool(createVersionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectKey, err := request.RequireString("project_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.CreateVersion(
			projectKey,
			name,
			request.GetString("description", ""),
			request.GetString("start_date", ""),
			request.GetString("release_date", ""),
			request.GetBool("released", false),
		))
	})
}
