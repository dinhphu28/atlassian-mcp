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

// bodyArgument returns the body to publish and whether the caller supplied one
// at all. Both content and file_path are optional, and an absent body must not
// be treated as an empty one: publishing "" replaces the page with a blank
// body, which is never what a caller who simply omitted the argument meant.
func bodyArgument(request mcp.CallToolRequest) (string, bool, error) {
	if filePath := request.GetString("file_path", ""); filePath != "" {
		content, err := readContent("", filePath)
		return content, true, err
	}

	content := request.GetString("content", "")
	return content, content != "", nil
}

func registerConfluenceWriteTools(s *server.MCPServer, client *confluence.Client) {
	createPageTool := mcp.NewTool(
		"confluence_create_page",
		mcp.WithDescription("Create a new Confluence page. Body is Markdown by default; "+
			"```mermaid blocks are rendered to images when mmdc is installed."),
		mcp.WithString("space_key", mcp.Required(), mcp.Description("Key of the space to create the page in")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Page title")),
		mcp.WithString("content", mcp.Description("Page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local file to publish instead of inline content. "+
			"Read in the chosen representation: Markdown by default, Confluence storage XHTML when representation='storage'.")),
		mcp.WithString("parent_id", mcp.Description("Optional parent page ID to nest under")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage, reprWiki),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, converted by this server and lossy), "+
				"'storage' (Confluence storage XHTML, sent verbatim - use it for macros, panels, layouts, "+
				"expand blocks, status lozenges and task lists), or 'wiki' (legacy wiki markup)")),
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
		content, hasBody, err := bodyArgument(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !hasBody {
			return mcp.NewToolResultError("one of 'content' or 'file_path' is required"), nil
		}
		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage, reprWiki)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		parentID := request.GetString("parent_id", "")
		if repr == reprMarkdown {
			return markdownPageResult(client.CreatePageMarkdown(spaceKey, title, content, parentID, mermaidRenderer()))
		}
		return jsonResult(client.CreatePage(spaceKey, title, content, parentID, repr))
	})

	updatePageTool := mcp.NewTool(
		"confluence_update_page",
		mcp.WithDescription("Update an existing Confluence page (version is bumped automatically). "+
			"Body is Markdown by default; ```mermaid blocks are rendered to images when mmdc is installed. "+
			"The body is replaced wholesale, so to preserve macros and other Confluence-native markup read the "+
			"page with representation='storage', edit that XHTML, and write it back with representation='storage'."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID")),
		mcp.WithString("content", mcp.Description("New page body in the given representation (Markdown by default). Ignored when file_path is set.")),
		mcp.WithString("file_path", mcp.Description("Optional path to a local file to publish instead of inline content. "+
			"Read in the chosen representation: Markdown by default, Confluence storage XHTML when representation='storage'.")),
		mcp.WithString("title", mcp.Description("New title (keeps the existing title if omitted). "+
			"Passing only a title renames the page and leaves its body untouched.")),
		mcp.WithNumber("expected_version", mcp.Description("Version this edit is based on (the version.number from your read). "+
			"When set, the write is rejected if the content changed since that read instead of overwriting it; "+
			"omit to always win the race.")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage, reprWiki),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, converted by this server and lossy), "+
				"'storage' (Confluence storage XHTML, sent verbatim - use it for macros, panels, layouts, "+
				"expand blocks, status lozenges and task lists), or 'wiki' (legacy wiki markup)")),
	)

	s.AddTool(updatePageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pageID, err := request.RequireString("page_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, hasBody, err := bodyArgument(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage, reprWiki)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		title := request.GetString("title", "")
		expectedVersion := request.GetInt("expected_version", 0)

		// No body given: rename if that is what was asked for, rather than
		// publishing an empty one over the page.
		if !hasBody {
			if title == "" {
				return mcp.NewToolResultError("one of 'content', 'file_path' or 'title' is required"), nil
			}
			return jsonResult(client.RenamePage(pageID, title))
		}

		if repr == reprMarkdown {
			return markdownPageResult(client.UpdatePageMarkdownAt(pageID, content, title, mermaidRenderer(), expectedVersion))
		}
		return jsonResult(client.UpdatePageAt(pageID, content, title, repr, expectedVersion))
	})

	addCommentTool := mcp.NewTool(
		"confluence_add_comment",
		mcp.WithDescription("Add a comment to a Confluence page. Body is Markdown by default."),
		mcp.WithString("page_id", mcp.Required(), mcp.Description("Confluence page ID to comment on")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Comment body in the given representation (Markdown by default)")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage, reprWiki),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, converted by this server and lossy), "+
				"'storage' (Confluence storage XHTML, sent verbatim - use it for macros, panels, layouts, "+
				"expand blocks, status lozenges and task lists), or 'wiki' (legacy wiki markup)")),
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

		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage, reprWiki)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if repr == reprMarkdown {
			return jsonResult(client.AddCommentMarkdown(pageID, content))
		}
		return jsonResult(client.AddComment(pageID, content, repr))
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
		mcp.WithNumber("expected_version", mcp.Description("Version this edit is based on (the version.number from your read). "+
			"When set, the write is rejected if the content changed since that read instead of overwriting it; "+
			"omit to always win the race.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("New comment body in the given representation (Markdown by default)")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage, reprWiki),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, converted by this server and lossy), "+
				"'storage' (Confluence storage XHTML, sent verbatim - use it for macros, panels, layouts, "+
				"expand blocks, status lozenges and task lists), or 'wiki' (legacy wiki markup)")),
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

		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage, reprWiki)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		expectedVersion := request.GetInt("expected_version", 0)
		if repr == reprMarkdown {
			return jsonResult(client.UpdateCommentMarkdownAt(commentID, content, expectedVersion))
		}
		return jsonResult(client.UpdateCommentAt(commentID, content, repr, expectedVersion))
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
		mcp.WithDescription("Upload a local file as an attachment on a Confluence page. "+
			"An attachment of the same name is updated in place (stored as a new version), "+
			"so page markup referencing that filename keeps working."),
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

	deleteAttachmentTool := mcp.NewTool(
		"confluence_delete_attachment",
		mcp.WithDescription("Delete an attachment from a Confluence page (get the attachment id from confluence_get_attachments). "+
			"Use it to clean up attachments the page no longer references."),
		mcp.WithString("attachment_id", mcp.Required(), mcp.Description("Attachment content id from confluence_get_attachments")),
	)

	s.AddTool(deleteAttachmentTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attachmentID, err := request.RequireString("attachment_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := client.DeleteAttachment(attachmentID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted attachment %s", attachmentID)), nil
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
		mcp.WithNumber("expected_version", mcp.Description("Version this edit is based on (the version.number from your read). "+
			"When set, the write is rejected if the content changed since that read instead of overwriting it; "+
			"omit to always win the race.")),
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

		return jsonResult(client.MovePageAt(pageID, targetParentID, request.GetInt("expected_version", 0)))
	})

	replyToCommentTool := mcp.NewTool(
		"confluence_reply_to_comment",
		mcp.WithDescription("Reply to an existing Confluence comment. Body is Markdown by default."),
		mcp.WithString("parent_comment_id", mcp.Required(), mcp.Description("ID of the comment to reply to")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Reply body in the given representation (Markdown by default)")),
		mcp.WithString("representation",
			mcp.Enum(reprMarkdown, reprStorage, reprWiki),
			mcp.DefaultString(reprMarkdown),
			mcp.Description("Body format: 'markdown' (default, converted by this server and lossy), "+
				"'storage' (Confluence storage XHTML, sent verbatim - use it for macros, panels, layouts, "+
				"expand blocks, status lozenges and task lists), or 'wiki' (legacy wiki markup)")),
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

		repr, err := bodyRepresentation(request, reprMarkdown, reprStorage, reprWiki)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if repr == reprMarkdown {
			return jsonResult(client.ReplyToCommentMarkdown(parentCommentID, content))
		}
		return jsonResult(client.ReplyToComment(parentCommentID, content, repr))
	})
}
