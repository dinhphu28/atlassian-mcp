# Atlassian MCP

A small, zero-dependency MCP server (single Go binary) exposing Confluence and
Jira (Server/Data Center) tools over stdio. Jira is optional — its tools are
only registered when configured.

## Build

Linux:

```sh
make release VERSION=0.1.0            # -> dist/atlassian-mcp_0.1.0_linux_amd64.tar.gz
```

Windows:

```sh
make release-windows VERSION=0.1.0    # -> dist/atlassian-mcp_0.1.0_windows_amd64.zip
```

Both at once:

```sh
make release-all VERSION=0.1.0
```

Cross-compiling is pure Go (`CGO_ENABLED=0`), so the Windows binary can be
built from Linux/mac. `zip` is required for the Windows package.

## Publish a release

Releases are what `atlassian-mcp update` pulls from.

**Primary (CI):** push a `vX.Y.Z` tag — the GitHub Actions workflow
(`.github/workflows/release.yml`) builds all platform archives (linux
amd64/arm64, windows amd64, macOS amd64/arm64) and creates the release with
auto-generated notes:

```sh
git tag v0.2.0 && git push origin v0.2.0
```

**Local/manual alternative:** `make publish` builds the same archives and creates
the release from your machine (requires the [`gh`](https://cli.github.com) CLI
authenticated and the commit pushed):

```sh
make publish VERSION=0.2.0
```

## Installation

### Linux / macOS

Extract the tarball and run setup:

```sh
tar -xzf atlassian-mcp_0.1.0_linux_amd64.tar.gz
cd atlassian-mcp_0.1.0_linux_amd64
./atlassian-mcp setup
```

The binary is installed to `~/.local/bin/atlassian-mcp` and the config is
written to `~/.config/atlassian-mcp/config.yaml`.

### Windows

Extract the zip, then open **PowerShell as Administrator** and run setup:

```powershell
Expand-Archive atlassian-mcp_0.1.0_windows_amd64.zip
cd atlassian-mcp_0.1.0_windows_amd64
.\atlassian-mcp.exe setup
```

The binary is installed to `%USERPROFILE%\.local\bin\atlassian-mcp.exe` and
the config is written to `%USERPROFILE%\.config\atlassian-mcp\config.yaml`.

## Upgrading

### Self-update

Update to the latest GitHub release in place:

```sh
./atlassian-mcp update          # download & install the latest release
./atlassian-mcp update --check  # only report whether a newer version exists
```

`update` downloads the release asset for your OS/arch, extracts the binary, and
atomically replaces the installed one (it works even while the MCP server is
running — restart/reconnect the server afterwards to use the new version).

Requires releases to be published on [GitHub Releases](https://github.com/dinhphu28/atlassian-mcp/releases);
set `GITHUB_TOKEN` in the environment for a private repo or higher rate limits.

### Manual upgrade

Alternatively, extract a release archive and re-run `setup`:

```sh
./atlassian-mcp setup
```

`setup` is safe to re-run. When a config already exists it **reuses your URLs and
tokens** — it does not re-ask — and only reinstalls the binary and migrates the
config schema if needed. You'll see one of `Reused existing config`, `Migrated
existing config to version N`, or (first run) `Created new config`.

### Changing credentials

Use the per-product commands to (re)configure or refresh tokens:

```sh
./atlassian-mcp confluence setup   # (re)configure Confluence URL + token
./atlassian-mcp jira setup         # (re)configure Jira URL + token
./atlassian-mcp confluence login   # refresh just the Confluence token (keeps URL)
./atlassian-mcp jira login         # refresh just the Jira token (keeps URL)
```

At the prompts, pressing Enter keeps the current value (the existing token is
preserved on a blank line).

### Config versioning

The config file carries a `config_version` field so upgrades can migrate it
automatically instead of asking you to reconfigure:

```yaml
config_version: 1
confluence:
  url: https://your-confluence-server.com
  pat: <your-token>
```

- Configs written by older releases (without `config_version`) are migrated on
  the next `setup`.
- The server refuses to start against a config whose `config_version` is *newer*
  than the binary supports, telling you to upgrade — so a downgrade can't
  silently misread a newer file.

## opencode config

`setup` prints the exact snippet to add, using the installed path. For example:

```yaml
"confluence":
  {
    "type": "local",
    "command": ["/home/<YOUR_HOME>/.local/bin/atlassian-mcp"],
  }
```

On Windows the command path will be the `.exe`, e.g.
`["C:\\Users\\<you>\\.local\\bin\\atlassian-mcp.exe"]`.

## Tools

All tools are read-only and authenticate with the configured Personal Access
Token.

### Markdown workflow (token-efficient)

Confluence page and comment tools use **Markdown** by default instead of raw
storage XHTML, cutting token usage on both reads and writes.

- **Write inline:** pass Markdown as `content` to `confluence_create_page` /
  `confluence_update_page`.
- **Write from a file:** pass `file_path` to publish a local `.md` file. Ideal
  for large pages — edit the file surgically, then push.
- **Read to a file:** pass `output_path` to `confluence_get_page` to write the
  Markdown to disk (returns metadata only), so large pages never fill context.
- **Exact fidelity:** pass `representation="storage"` on reads/writes to bypass
  Markdown conversion.

**Images:** `![alt](name.png)` references a page attachment (upload it first
with `confluence_upload_attachment`); `![alt](https://…)` embeds an external URL.

**Mermaid:** ` ```mermaid ` blocks are rendered to PNG and uploaded as
attachments **if the `mmdc` CLI (`@mermaid-js/mermaid-cli`) is on PATH**. Without
it, the block is stored as a code macro. Rendering is one-way — reads return the
image, not the Mermaid source.

### `confluence_search`

Search Confluence content.

| Param   | Required | Description                                                              |
| ------- | -------- | ------------------------------------------------------------------------ |
| `query` | no\*     | Free-text keyword, matched as CQL `text ~ "query"`.                      |
| `cql`   | no\*     | Raw CQL expression, e.g. `space = DEV AND label = api`. Overrides `query`. |
| `limit` | no       | Maximum number of results (default `10`).                                |

\* Provide either `query` or `cql`.

### `confluence_get_page`

Get a single page by ID (includes `body.storage`, space, and version).

| Param     | Required | Description           |
| --------- | -------- | --------------------- |
| `page_id` | yes      | Confluence page ID.   |

### `confluence_get_page_children`

List the child pages directly under a page.

| Param     | Required | Description                              |
| --------- | -------- | ---------------------------------------- |
| `page_id` | yes      | Parent Confluence page ID.               |
| `limit`   | no       | Maximum number of children (default `25`). |

### `confluence_get_comments`

Get the comments on a page (includes `body.storage`).

| Param     | Required | Description                              |
| --------- | -------- | ---------------------------------------- |
| `page_id` | yes      | Confluence page ID.                      |
| `limit`   | no       | Maximum number of comments (default `25`). |

## Confluence access token

Get a Personal Access Token from:

`https://your-confluence-server.com/plugins/personalaccesstokens/usertokens.action`
