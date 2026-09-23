package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
)

func registerConfluenceContentReadTools(s *server.MCPServer, client *confluence.Client) {
	getDescendantsTool := mcp.NewTool(
		"confluence_get_descendants",
		mcp.WithDescription("List every page in the subtree under a Confluence page, at any depth. "+
			"Use confluence_get_page_children instead when only the direct children are wanted. "+
			"Results are paged: pass 'start' to continue past 'limit'."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID at the top of the subtree")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of descendants (default 50)")),
		mcp.WithNumber("start", mcp.Description("Index of the first result, for paging (default 0)")),
	)

	s.AddTool(getDescendantsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetDescendants(pageID, request.GetInt("limit", 50), request.GetInt("start", 0)))
	})

	getAncestorsTool := mcp.NewTool(
		"confluence_get_ancestors",
		mcp.WithDescription("Get the breadcrumb trail of a Confluence page: its ancestors as {id, title}, "+
			"ordered from the space root down to its direct parent. The page itself is not included, "+
			"and an empty list means the page is at the root of its space."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
	)

	s.AddTool(getAncestorsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetAncestors(pageID))
	})

	getRestrictionsTool := mcp.NewTool(
		"confluence_get_restrictions",
		mcp.WithDescription("Get a Confluence page's restrictions grouped by operation: who may read it and "+
			"who may update it. An operation with no users and no groups is unrestricted, so access to it "+
			"follows the space permissions. Inherited restrictions from parent pages are not listed here - "+
			"walk confluence_get_ancestors to see those."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
	)

	s.AddTool(getRestrictionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetRestrictions(pageID))
	})

	getPropertiesTool := mcp.NewTool(
		"confluence_get_properties",
		mcp.WithDescription("Get the content properties of a Confluence page - arbitrary JSON stored against "+
			"the page under a key, which is where tooling keeps its own state. Omit 'key' to list them all."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("key", mcp.Description("Property key to fetch (omit to list every property on the page)")),
	)

	s.AddTool(getPropertiesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetProperties(pageID, request.GetString("key", "")))
	})

	isWatchingTool := mcp.NewTool(
		"confluence_is_watching_page",
		mcp.WithDescription("Report whether the user this server's token belongs to is watching a Confluence "+
			"page. Only that user's own watch state is visible; the full list of a page's watchers is not "+
			"exposed by the Server/Data Center REST API. Change it with confluence_watch_page."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
	)

	s.AddTool(isWatchingTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.IsWatchingPage(pageID))
	})

	getTemplatesTool := mcp.NewTool(
		"confluence_get_templates",
		mcp.WithDescription("List the page templates of a space, or one template's storage-format body when "+
			"'template_id' is given (the ids come from the listing). Confluence Server/Data Center has no "+
			"\"create page from template\" REST call: to use a template, read its body here and pass that "+
			"body to confluence_create_page with representation='storage'. Note that a template body may "+
			"contain <at:declarations> variable placeholders, which you should fill in or strip first."),
		mcp.WithString("space_key", mcp.Description("Space key to list templates of (omit for the global templates)")),
		mcp.WithString("template_id", mcp.Description("ID of a single template to fetch, including its body. Overrides 'space_key'.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of templates (default 25)")),
		mcp.WithNumber("start", mcp.Description("Index of the first result, for paging (default 0)")),
	)

	s.AddTool(getTemplatesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if templateID := request.GetString("template_id", ""); templateID != "" {
			return jsonResult(client.GetTemplate(templateID))
		}

		return jsonResult(client.GetTemplates(
			request.GetString("space_key", ""),
			request.GetInt("limit", 25),
			request.GetInt("start", 0)))
	})
}

func registerConfluenceContentWriteTools(s *server.MCPServer, client *confluence.Client) {
	deleteLabelTool := mcp.NewTool(
		"confluence_delete_label",
		mcp.WithDescription("Remove a label from a Confluence page. Get the current labels from "+
			"confluence_get_labels; the name is the bare label, without its prefix."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Label name to remove, e.g. 'api' (not 'global:api')")),
	)

	s.AddTool(deleteLabelTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteLabel(pageID, name); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Removed label %q from page %s", name, pageID)), nil
	})

	setRestrictionsTool := mcp.NewTool(
		"confluence_set_restrictions",
		mcp.WithDescription("Set who may read and who may update a Confluence page. Each argument is a "+
			"comma-separated list of Server/Data Center usernames or group names (not email addresses or "+
			"account ids). An argument you omit leaves that part of the page's restrictions exactly as it is; "+
			"pass the literal 'none' to restrict nobody for it, which makes the operation follow the space "+
			"permissions again. Restricting read access can lock other people out of the page, so read "+
			"confluence_get_restrictions first and include the people who are already on it."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("read_users", mcp.Description("Usernames allowed to view, comma-separated; 'none' to clear; omit to keep the current ones")),
		mcp.WithString("read_groups", mcp.Description("Group names allowed to view, comma-separated; 'none' to clear; omit to keep the current ones")),
		mcp.WithString("update_users", mcp.Description("Usernames allowed to edit, comma-separated; 'none' to clear; omit to keep the current ones")),
		mcp.WithString("update_groups", mcp.Description("Group names allowed to edit, comma-separated; 'none' to clear; omit to keep the current ones")),
	)

	s.AddTool(setRestrictionsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		readUsers := request.GetString("read_users", "")
		readGroups := request.GetString("read_groups", "")
		updateUsers := request.GetString("update_users", "")
		updateGroups := request.GetString("update_groups", "")

		if readUsers == "" && readGroups == "" && updateUsers == "" && updateGroups == "" {
			return mcp.NewToolResultError("give at least one of read_users, read_groups, update_users or update_groups " +
				"(use 'none' to clear one)"), nil
		}

		return jsonResult(client.SetRestrictions(pageID, readUsers, readGroups, updateUsers, updateGroups))
	})

	setPropertyTool := mcp.NewTool(
		"confluence_set_property",
		mcp.WithDescription("Store a JSON content property against a Confluence page, creating it or "+
			"replacing it. The value replaces the property wholesale, so read the current one with "+
			"confluence_get_properties and merge before writing if you only mean to change a field."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Property key, e.g. 'my-tool-state'")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Property value as JSON, e.g. {\"reviewed\":true,\"by\":\"alice\"}. "+
			"Objects, arrays, strings, numbers and booleans are all valid; a bare word is not.")),
	)

	s.AddTool(setPropertyTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := request.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		value, err := request.RequireString("value")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.SetProperty(pageID, key, value))
	})

	deletePropertyTool := mcp.NewTool(
		"confluence_delete_property",
		mcp.WithDescription("Delete a content property from a Confluence page. Get the key from confluence_get_properties."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Property key to delete")),
	)

	s.AddTool(deletePropertyTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := request.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteProperty(pageID, key); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted property %q from page %s", key, pageID)), nil
	})

	watchPageTool := mcp.NewTool(
		"confluence_watch_page",
		mcp.WithDescription("Start or stop watching a Confluence page, so the user this server's token "+
			"belongs to is notified of changes to it. Only that user's own watch state can be changed. "+
			"Check it with confluence_is_watching_page."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithBoolean("watch", mcp.DefaultBool(true), mcp.Description("true to watch the page (default), false to stop watching it")),
	)

	s.AddTool(watchPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		message, err := client.WatchPage(pageID, request.GetBool("watch", true))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(message), nil
	})

	copyPageTool := mcp.NewTool(
		"confluence_copy_page",
		mcp.WithDescription("Copy a Confluence page into a new page. The copy is made by re-publishing the "+
			"source page's storage-format body, so ONLY the body travels: attachments, child pages, labels, "+
			"restrictions and comments are NOT copied, and any image or file the body references stays "+
			"pointing at the original page's attachments until you upload those files to the copy with "+
			"confluence_upload_attachment. Defaults to the source page's own space and a title of "+
			"\"Copy of <title>\", since Confluence requires titles to be unique within a space. "+
			"A blog post is copied as a blog post, which is why target_parent_id is refused for one."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("ID of the page to copy")),
		mcp.WithString("target_space_key", mcp.Description("Space key to copy into (defaults to the source page's space)")),
		mcp.WithString("target_parent_id", mcp.Description("Page ID to nest the copy under (defaults to the root of the target space)")),
		mcp.WithString("new_title", mcp.Description("Title for the copy (defaults to \"Copy of <source title>\")")),
	)

	s.AddTool(copyPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.CopyPage(
			pageID,
			request.GetString("target_space_key", ""),
			request.GetString("target_parent_id", ""),
			request.GetString("new_title", "")))
	})
}
