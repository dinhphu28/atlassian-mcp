package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/jira"
)

func registerJiraSocialReadTools(s *server.MCPServer, client *jira.Client) {
	getWatchersTool := mcp.NewTool(
		"jira_get_watchers",
		mcp.WithDescription("List who is watching a Jira issue (their usernames are what jira_add_watcher / jira_remove_watcher take)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getWatchersTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetWatchers(key))
	})

	getVotesTool := mcp.NewTool(
		"jira_get_votes",
		mcp.WithDescription("Get the vote count on a Jira issue, and whether the current user has voted"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getVotesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetVotes(key))
	})

	getRemoteLinksTool := mcp.NewTool(
		"jira_get_remote_links",
		mcp.WithDescription("List a Jira issue's remote links - its links out to Confluence pages, web pages and other applications. Each entry's 'id' is the link_id for jira_delete_remote_link, and its 'globalId' is the global_id."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(getRemoteLinksTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.GetRemoteLinks(key))
	})
}

func registerJiraSocialWriteTools(s *server.MCPServer, client *jira.Client) {
	addWatcherTool := mcp.NewTool(
		"jira_add_watcher",
		mcp.WithDescription("Add a user as a watcher of a Jira issue. username is a Jira Server/Data Center username, not an email address; see jira_get_watchers for the form usernames take on this instance."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("username", mcp.Required(), mcp.Description("Jira username to add, e.g. jsmith")),
	)

	s.AddTool(addWatcherTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		username, err := request.RequireString("username")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		text, err := client.AddWatcher(key, username)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(text), nil
	})

	removeWatcherTool := mcp.NewTool(
		"jira_remove_watcher",
		mcp.WithDescription("Remove a user from a Jira issue's watchers (get the username from jira_get_watchers)"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("username", mcp.Required(), mcp.Description("Jira username to remove, e.g. jsmith")),
	)

	s.AddTool(removeWatcherTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		username, err := request.RequireString("username")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.RemoveWatcher(key, username); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Removed %s from the watchers of %s", username, key)), nil
	})

	voteTool := mcp.NewTool(
		"jira_vote",
		mcp.WithDescription("Vote for a Jira issue as the user the API token belongs to. Jira refuses a vote on an issue that user reported."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(voteTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		text, err := client.Vote(key)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(text), nil
	})

	unvoteTool := mcp.NewTool(
		"jira_unvote",
		mcp.WithDescription("Withdraw the API token user's vote from a Jira issue"),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
	)

	s.AddTool(unvoteTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		text, err := client.Unvote(key)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(text), nil
	})

	addRemoteLinkTool := mcp.NewTool(
		"jira_add_remote_link",
		mcp.WithDescription("Link a Jira issue out to a URL, typically a Confluence page (get the page URL from confluence_get_page or confluence_search). Jira upserts on global_id, so calling this twice with the same global_id updates the existing link instead of duplicating it; when global_id is omitted it defaults to the url, which makes re-linking the same page idempotent."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL, e.g. https://confluence.example.com/display/DOC/Release+Notes")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Link text shown on the issue, e.g. the Confluence page title")),
		mcp.WithString("summary", mcp.Description("Optional one-line description shown under the link")),
		mcp.WithString("relationship", mcp.Description("Optional relationship shown as the link's heading, e.g. documentation, mentioned in, caused by")),
		mcp.WithString("global_id", mcp.Description("Optional stable identifier for this link; defaults to the url. Re-posting the same global_id updates that link.")),
		mcp.WithString("icon_url", mcp.Description("Optional 16x16 icon URL shown beside the link")),
		mcp.WithString("icon_title", mcp.Description("Optional icon tooltip; ignored unless icon_url is given")),
	)

	s.AddTool(addRemoteLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		linkURL, err := request.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.AddRemoteLink(
			key, linkURL, title,
			request.GetString("summary", ""),
			request.GetString("relationship", ""),
			request.GetString("global_id", ""),
			request.GetString("icon_url", ""),
			request.GetString("icon_title", ""),
		))
	})

	deleteRemoteLinkTool := mcp.NewTool(
		"jira_delete_remote_link",
		mcp.WithDescription("Delete a remote link from a Jira issue, addressed either by link_id or by global_id (get both from jira_get_remote_links). Provide one of them; link_id wins if both are given."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. DEV-123) or numeric ID")),
		mcp.WithString("link_id", mcp.Description("Remote link id from jira_get_remote_links")),
		mcp.WithString("global_id", mcp.Description("The link's globalId from jira_get_remote_links; for links made by jira_add_remote_link without a global_id this is the target url")),
	)

	s.AddTool(deleteRemoteLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := request.RequireString("issue_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		linkID := request.GetString("link_id", "")
		globalID := request.GetString("global_id", "")
		if err := client.DeleteRemoteLink(key, linkID, globalID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		target := linkID
		if target == "" {
			target = globalID
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted remote link %s on %s", target, key)), nil
	})
}
