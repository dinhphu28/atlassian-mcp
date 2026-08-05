package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dinhphu28/atlassian-mcp/internal/confluence"
	"dinhphu28/atlassian-mcp/internal/mermaid"
)

// mermaidRenderer returns a renderer when mmdc is available, else nil so that
// diagrams degrade to code macros.
func mermaidRenderer() confluence.MermaidRenderer {
	if !mermaid.Available() {
		return nil
	}
	return mermaid.Render
}

// readContent returns the body to publish: the file contents when filePath is
// set, otherwise the inline content.
func readContent(inline, filePath string) (string, error) {
	if filePath == "" {
		return inline, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func registerConfluenceWriteTools(s *server.MCPServer, client *confluence.Client) {
	createPageTool := mcp.NewTool(
		"confluence_create_page",
		mcp.WithDescription("Create a new Confluence page. Body is Markdown by default; "+
			"```mermaid blocks are rendered to images when mmdc is installed."),
		mcp.WithString("space_key", mcp.Required(), mcp.Description("Key of the space to create the page in")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Page title")),
		mcp.WithString("content", mcp.Description("Page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local Markdown file to publish instead of inline content")),
		mcp.WithString("parent_id", mcp.Description("Optional parent page ID to nest under")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(createPageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		spaceKey, err := request.RequireString("space_key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := request.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := readContent(request.GetString("content", ""), request.GetString("file_path", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		parentID := request.GetString("parent_id", "")
		if representation == "markdown" {
			return jsonResult(client.CreatePageMarkdown(spaceKey, title, content, parentID, mermaidRenderer()))
		}
		return jsonResult(client.CreatePage(spaceKey, title, content, parentID, representation))
	})

	updatePageTool := mcp.NewTool(
		"confluence_update_page",
		mcp.WithDescription("Update an existing Confluence page (version is bumped automatically). "+
			"Body is Markdown by default; ```mermaid blocks are rendered to images when mmdc is installed."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("content", mcp.Description("New page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local Markdown file to publish instead of inline content")),
		mcp.WithString("title", mcp.Description("New title (keeps the existing title if omitted)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(updatePageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := readContent(request.GetString("content", ""), request.GetString("file_path", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		title := request.GetString("title", "")
		if representation == "markdown" {
			return jsonResult(client.UpdatePageMarkdown(pageID, content, title, mermaidRenderer()))
		}
		return jsonResult(client.UpdatePage(pageID, content, title, representation))
	})

	addCommentTool := mcp.NewTool(
		"confluence_add_comment",
		mcp.WithDescription("Add a comment to a Confluence page. Body is Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID to comment on")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Comment body in the given representation (Markdown by default)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(addCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		if representation == "markdown" {
			return jsonResult(client.AddCommentMarkdown(pageID, content))
		}
		return jsonResult(client.AddComment(pageID, content, representation))
	})

	deletePageTool := mcp.NewTool(
		"confluence_delete_page",
		mcp.WithDescription("Delete a Confluence page by ID (moves it to the trash)"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID to delete")),
	)

	s.AddTool(deletePageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeletePage(pageID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted page %s", pageID)), nil
	})

	updateCommentTool := mcp.NewTool(
		"confluence_update_comment",
		mcp.WithDescription("Edit an existing Confluence comment (version is bumped automatically). "+
			"Body is Markdown by default. Get the comment id from confluence_get_comments."),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("ID of the comment to edit")),
		mcp.WithString("content", mcp.Required(), mcp.Description("New comment body in the given representation (Markdown by default)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(updateCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		commentID, err := request.RequireString("comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		if representation == "markdown" {
			return jsonResult(client.UpdateCommentMarkdown(commentID, content))
		}
		return jsonResult(client.UpdateComment(commentID, content, representation))
	})

	deleteCommentTool := mcp.NewTool(
		"confluence_delete_comment",
		mcp.WithDescription("Delete a Confluence comment by ID"),
		mcp.WithString("comment_id", mcp.Required(), mcp.Description("ID of the comment to delete")),
	)

	s.AddTool(deleteCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		commentID, err := request.RequireString("comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteComment(commentID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted comment %s", commentID)), nil
	})

	uploadAttachmentTool := mcp.NewTool(
		"confluence_upload_attachment",
		mcp.WithDescription("Upload a local file as an attachment on a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Page ID to attach the file to")),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the local file to upload")),
	)

	s.AddTool(uploadAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filePath, err := request.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.UploadAttachment(pageID, filepath.Base(filePath), data))
	})

	addLabelTool := mcp.NewTool(
		"confluence_add_label",
		mcp.WithDescription("Add a label to a Confluence page"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Label name")),
	)

	s.AddTool(addLabelTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := request.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.AddLabel(pageID, name))
	})

	movePageTool := mcp.NewTool(
		"confluence_move_page",
		mcp.WithDescription("Move a Confluence page under a new parent (title and content preserved)"),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("ID of the page to move")),
		mcp.WithString("target_parent_id", mcp.Required(), mcp.Description("ID of the new parent page")),
	)

	s.AddTool(movePageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		targetParentID, err := request.RequireString("target_parent_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(client.MovePage(pageID, targetParentID))
	})

	replyToCommentTool := mcp.NewTool(
		"confluence_reply_to_comment",
		mcp.WithDescription("Reply to an existing Confluence comment. Body is Markdown by default."),
		mcp.WithString("parent_comment_id", mcp.Required(), mcp.Description("ID of the comment to reply to")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Reply body in the given representation (Markdown by default)")),
		mcp.WithString("representation", mcp.Description("Body format: 'markdown' (default), 'storage', or 'wiki'")),
	)

	s.AddTool(replyToCommentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parentCommentID, err := request.RequireString("parent_comment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := request.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		representation := request.GetString("representation", "markdown")
		if representation == "markdown" {
			return jsonResult(client.ReplyToCommentMarkdown(parentCommentID, content))
		}
		return jsonResult(client.ReplyToComment(parentCommentID, content, representation))
	})
}
