# MyKB

Personal knowledge base with full-text and semantic search, exposed via [MCP](https://modelcontextprotocol.io/) (Model Context Protocol).

Store text chunks with metadata, search them with FTS5 or vector similarity, and let Claude (or any MCP client) access your knowledge.

**Single-user by design.** One password, one database, no user management. Perfect for personal notes, bookmarks, code snippets, or anything you want to remember.

## Features

- **Full-text search** with SQLite FTS5 (supports wildcards, OR, phrases)
- **Semantic search** with OpenAI or Ollama embeddings
- **MCP server** for Claude Desktop, Claude Code, or any MCP client
- **Self-hosted** single binary, no external dependencies
- **HTTPS** with automatic Let's Encrypt certificates
- **OAuth 2.0** with PKCE and dynamic registration

## Installation

```bash
go install github.com/neoden/mykb@latest
```

Or build from source:

```bash
git clone https://github.com/neoden/mykb
cd mykb
go build -o mykb .
```

## Quick Start

### Local (stdio)

```
mykb serve stdio  
```

### Remote (HTTPS)

1. Create config at `~/.config/mykb/config.toml`:

```toml
[server]
domain = "mykb.example.com"

[embedding]
provider = "openai"

[embedding.openai]
api_key = "sk-..."
```

2. Set password and start:

```bash
mykb set-password
mykb serve http
```

3. Add MCP server entry to your IDE or any other client application:

```json
{
  "mcpServers": {
    "mykb": {
      "url": "https://mykb.example.com/mcp"
    }
  }
}
```

## Configuration

Config file is searched in order:

| Platform | Paths |
|----------|-------|
| Linux/macOS | `~/.config/mykb/config.toml`, `/etc/mykb/config.toml` |
| Windows | `%APPDATA%\mykb\config.toml` |

Override with `--config`:

```bash
mykb --config /path/to/config.toml serve http
```

Example config:

```toml
data_dir = "/var/lib/mykb"  # default: ~/.local/share/mykb

[server]
listen = ":8080"            # HTTP on localhost (dev)
# domain = "mykb.example.com" # HTTPS with auto TLS (prod)
behind_proxy = false        # Trust X-Forwarded-For

[embedding]
provider = "openai"         # "openai" or "ollama"

[embedding.openai]
api_key = "sk-..."
model = "text-embedding-3-small"  # default

[embedding.ollama]
url = "http://localhost:11434"    # default
model = "nomic-embed-text"        # default
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `store_chunk` | Store text with optional metadata |
| `search_chunks` | Full-text search (FTS5 syntax) |
| `semantic_search` | Vector similarity search |
| `get_chunk` | Get chunk by ID |
| `update_chunk` | Update content or metadata |
| `delete_chunk` | Delete chunk |
| `get_metadata_index` | Overview of all metadata keys/values |
| `get_metadata_values` | Drill down into specific metadata key |

### Search Syntax

FTS5 query syntax is supported:

```
hello world     # AND (both words)
hello OR world  # OR
"hello world"   # exact phrase
hello*          # prefix wildcard
*               # all chunks
```

## CLI Commands

```bash
mykb serve stdio          # MCP over stdio (local)
mykb serve http           # HTTP server
mykb set-password         # Set auth password
mykb reindex [--force]    # Generate embeddings for existing chunks
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o mykb .
```

## License

MIT
