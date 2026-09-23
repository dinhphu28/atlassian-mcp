package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
)

func registerConfluenceSpaceReadTools(s *server.MCPServer, client *confluence.Client) {
	getSpacesTool := mcp.NewTool(
		"confluence_get_spaces",
		mcp.WithDescription("List the Confluence spaces on this instance. Use it to discover the space key "+
			"that confluence_create_page, confluence_search (space = KEY) and the other space tools need. "+
			"Compact by default: each space is reduced to key, name, type and id."),
		mcp.WithString("type",
			mcp.Enum("global", "personal"),
			mcp.Description("Only spaces of this type: 'global' (team spaces) or 'personal' (~username spaces). Both when omitted.")),
		mcp.WithString("status",
			mcp.Enum("current", "archived"),
			mcp.Description("Only spaces in this state: 'current' or 'archived' (default: whatever Confluence returns, normally current)")),
		mcp.WithString("label", mcp.Description("Only spaces carrying this space label")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of spaces (default 25)")),
		mcp.WithNumber("start", mcp.Description("Index of the first space to return, for paging (default 0)")),
		mcp.WithBoolean("compact", mcp.Description("Reduce each space to {key, name, type, id} (default true). "+
			"Set false for the full REST payload including links and expandable fields.")),
	)

	s.AddTool(getSpacesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetSpaces(
			request.GetString("type", ""),
			request.GetString("status", ""),
			request.GetString("label", ""),
			request.GetInt("limit", 25),
			request.GetInt("start", 0),
			request.GetBool("compact", true),
		))
	})

	getSpaceTool := mcp.NewTool(
		"confluence_get_space",
		mcp.WithDescription("Get one Confluence space by key, with its description, home page and labels. "+
			"The homepage id in the response is the root of the space's page tree: pass it to "+
			"confluence_get_page_children to browse the space from the top. Get the key from confluence_get_spaces."),
		mcp.WithString("space_key", mcp.Required(), mcp.Description("Space key, e.g. DEV (or ~username for a personal space)")),
	)

	s.AddTool(getSpaceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		spaceKey, err := request.RequireString("space_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetSpace(spaceKey))
	})

	getSpaceContentTool := mcp.NewTool(
		"confluence_get_space_content",
		mcp.WithDescription("List the top-level content of a Confluence space (the pages and blog posts at its root, "+
			"not the whole tree - descend with confluence_get_page_children). Get the key from confluence_get_spaces."),
		mcp.WithString("space_key", mcp.Required(), mcp.Description("Space key, e.g. DEV")),
		mcp.WithString("type",
			mcp.Enum("page", "blogpost"),
			mcp.Description("Only content of this type: 'page' or 'blogpost'. Both when omitted.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of items (default 25)")),
		mcp.WithNumber("start", mcp.Description("Index of the first item to return, for paging (default 0)")),
	)

	s.AddTool(getSpaceContentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		spaceKey, err := request.RequireString("space_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetSpaceContent(
			spaceKey,
			request.GetString("type", ""),
			request.GetInt("limit", 25),
			request.GetInt("start", 0),
		))
	})

	getCurrentUserTool := mcp.NewTool(
		"confluence_get_current_user",
		mcp.WithDescription("Get the Confluence user this server authenticates as (the owner of the Personal "+
			"Access Token). Its username is what CQL fields such as creator and mention expect."),
	)

	s.AddTool(getCurrentUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(client.GetCurrentUser())
	})

	getUserTool := mcp.NewTool(
		"confluence_get_user",
		mcp.WithDescription("Get a Confluence user by username. This is Server/Data Center, which identifies "+
			"users by username, not by the accountId used on Confluence Cloud. "+
			"Find a username with confluence_search_users."),
		mcp.WithString("username", mcp.Required(), mcp.Description("Confluence username, e.g. jdoe")),
	)

	s.AddTool(getUserTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		username, err := request.RequireString("username")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetUser(username))
	})

	searchUsersTool := mcp.NewTool(
		"confluence_search_users",
		mcp.WithDescription("Find Confluence users whose full name matches a query, to get the username "+
			"confluence_get_user and CQL need. Runs CQL 'user.fullname ~ \"query\"', trying the dedicated "+
			"/rest/api/search/user endpoint first and falling back to /rest/api/search, because which one "+
			"serves user fields varies by Server/Data Center version. Some versions support neither, so if "+
			"this reports that user search is unavailable, look the user up by name with confluence_get_user."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Part of a user's full name, e.g. 'Jane'")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of users (default 25)")),
	)

	s.AddTool(searchUsersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.SearchUsers(query, request.GetInt("limit", 25)))
	})
}

func registerConfluenceSpaceWriteTools(s *server.MCPServer, client *confluence.Client) {
	createSpaceTool := mcp.NewTool(
		"confluence_create_space",
		mcp.WithDescription("Create a new Confluence space. The key must be letters and digits only and is "+
			"uppercased, the convention for space keys. A private space is visible only to its creator until "+
			"they grant access to others."),
		mcp.WithString("key", mcp.Required(), mcp.Description("Space key: letters and digits only, e.g. DEV. "+
			"Must not already exist - check with confluence_get_spaces.")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Space name, e.g. 'Developer Docs'")),
		mcp.WithString("description", mcp.Description("Optional plain-text description of the space")),
		mcp.WithBoolean("private", mcp.Description("Create a private space, visible only to you (default false)")),
	)

	s.AddTool(createSpaceTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.CreateSpace(
			key,
			name,
			request.GetString("description", ""),
			request.GetBool("private", false),
		))
	})
}
