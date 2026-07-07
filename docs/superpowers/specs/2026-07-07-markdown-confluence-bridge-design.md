# Markdown ↔ Confluence Bridge — Design

**Date:** 2026-07-07
**Status:** Approved (pending implementation plan)
**Component:** `atlassian-mcp` — Confluence tools

## Problem

Writing and updating Confluence pages currently forces the AI agent to author
and read Confluence **storage format** — verbose XHTML with special macros
(`<ac:structured-macro>`, `<ac:image>`, `<ac:link>`). This is expensive on two
axes:

- **Writes** burn output tokens producing verbose XHTML.
- **Reads** burn input tokens: `get_page` returns `body.storage` XHTML.

Markdown is ~2–3× more compact than storage XHTML. Converting **inside the Go
server** (Markdown ↔ storage) attacks both axes. A file-based path additionally
lets the agent make surgical edits with local tooling (`Edit`, `sed`, `grep`)
instead of re-emitting the whole page body on every update.

## Goals

- Accept and return **Markdown** as the default body representation for
  Confluence pages and comments.
- Keep exact-fidelity `storage` (and `wiki`) available as explicit opt-outs.
- Support both **inline** Markdown and a **file-based** path (the token-optimal
  update loop).
- Handle **images** losslessly across the round-trip.
- Render **Mermaid** diagrams to images locally so they display in Confluence
  without a Marketplace app.

## Non-Goals (v1)

- Wiki-markup bridge (Markdown → Confluence wiki → storage via REST convert).
- Kroki or any hosted diagram renderer.
- Mermaid **source** round-trip (reading a rendered diagram back as
  ` ```mermaid ` source). Reads return the rendered image reference.
- ADF (`atlas_doc_format`).

## Tool Interface

### Representation parameter

Reuse the existing `representation` parameter; add `"markdown"` as an accepted
value alongside `"storage"` and `"wiki"`.

- **Default = `markdown`** on both reads and writes.
- `representation="storage"` is the exact-fidelity opt-out; `"wiki"` unchanged.

Rationale for markdown-as-default on both sides: it makes the round-trip clean
(read as Markdown → edit Markdown → write Markdown). A split default (markdown
reads, storage writes) would force the agent to hand-translate on write,
defeating the purpose.

### Writes — `create_page`, `update_page`, `add_comment`, `reply_to_comment`

- Keep the inline `content` argument.
- Add an optional `file_path` argument. When set, the server reads the `.md`
  from disk and `content` is ignored.

### Reads — `get_page`, `get_comments`

- Return Markdown by default (convert `body.storage` → Markdown).
- Add an optional `output_path` argument. When set, write the Markdown to that
  file and return a short confirmation plus page metadata (id, version, title,
  space) instead of the body inline.

Token-optimal update loop:

1. `get_page(page_id, output_path="/tmp/page.md")` — nothing dumped into context.
2. Agent edits only the changed lines in the file (`Edit`/`sed`).
3. `update_page(page_id, file_path="/tmp/page.md")` — read + convert + push.

## Conversion Engine — `internal/markdown/`

New pure package: string in → string out, no Confluence coupling, independently
testable.

### Markdown → storage

- `github.com/yuin/goldmark` + GFM extension (tables, strikethrough, task
  lists).
- Custom renderer overriding two node types:
  - Fenced code block → `<ac:structured-macro ac:name="code">` with the language
    as a parameter.
  - Image → `<ac:image>` (see Images below).
- All other nodes (`<p>`, `<ul>`, `<ol>`, `<h1-6>`, `<strong>`, `<em>`, `<a>`,
  `<table>`) already emit valid storage XHTML.

### Storage → Markdown

- `github.com/JohannesKaufmann/html-to-markdown` with custom rules for:
  - `<ac:image>` → `![alt](filename-or-url)`
  - code macro → fenced code block with language
  - `<ac:link>` → Markdown link
- **Unknown `ac:` / `ri:` macros pass through as raw HTML** so nothing is
  silently dropped.

### New dependencies

- `github.com/yuin/goldmark`
- `github.com/JohannesKaufmann/html-to-markdown`

## Images

| Markdown | Storage |
|---|---|
| `![alt](diagram.png)` (bare filename → page attachment) | `<ac:image><ri:attachment ri:filename="diagram.png"/></ac:image>` |
| `![alt](https://…/x.png)` (URL) | `<ac:image><ri:url ri:value="https://…/x.png"/></ac:image>` |

Rules live in both the goldmark renderer and the html-to-markdown ruleset so the
round-trip is symmetric. A bare (schemeless) src is treated as an attachment
filename; anything with a scheme is treated as an external URL.

## Mermaid — `internal/mermaid/`

New pure package: `Render(source string) (png []byte, err error)`, shelling out
to the local `mmdc` (`@mermaid-js/mermaid-cli`) binary.

### Write flow

1. Detect ` ```mermaid ` fenced blocks during Markdown → storage conversion.
2. For each, render to PNG via `mmdc`.
3. Upload the PNG as a page attachment.
4. Replace the fence with `<ac:image>` referencing the attachment filename.

### Ordering

- `create_page` with diagrams: create page → upload attachment(s) → patch body
  to reference them (the page must exist before attachments can attach).
- `update_page`: upload attachment(s), then update the body in the existing
  version bump.

### Fallback

- If `mmdc` is not on `PATH`, emit a plain `code` macro (language=mermaid)
  instead — never fail the write. Log so the user knows rendering was skipped.

### Round-trip

- One-way: reads return the rendered image reference (`![alt](diagram.png)`), not
  the Mermaid source. Re-editing a diagram means regenerating the source.

## Layering

- `internal/markdown/` — Markdown ↔ storage conversion. Pure, no I/O.
- `internal/mermaid/` — Mermaid source → PNG bytes. Shells out to `mmdc`.
- `internal/confluence/pages.go` — orchestrates: convert → (render + upload
  Mermaid attachments) → POST/PUT with version bump.
- `internal/mcpserver/confluence_read.go` / `confluence_write.go` — `file_path` /
  `output_path` file I/O, updated tool descriptions advertising `markdown` as the
  default representation.

## Error Handling

- Conversion errors (malformed Markdown/XHTML) return a tool error with the
  offending context; the page is not written.
- `mmdc` failures on an individual diagram fall back to the code macro for that
  diagram and continue.
- Missing `file_path` / unreadable file returns a tool error before any API call.
- `output_path` write failures return a tool error; the fetched page is not lost
  (metadata still returned).

## Testing

- Table-driven unit tests for both converters: headings, lists, bold/italic,
  links, inline + fenced code, tables, images (attachment + URL), unknown-macro
  passthrough.
- **Round-trip** tests: `md → storage → md` for the common subset.
- Mermaid: fence detection + fallback path with `mmdc` absent/mocked; one
  integration test gated on `mmdc` being installed.
- File-path / output-path I/O tests at the mcpserver layer.
