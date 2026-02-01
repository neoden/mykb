# MyKB - Project Guide

## What is this?

A self-hosted text chunk storage and search service exposed via MCP (Model Context Protocol). It allows Claude to store and retrieve knowledge chunks with full-text search.

## Architecture

```
Internet → mykb (:443) → SQLite
              ↓
         MCP (/mcp)
```

Single Go binary with:
- **Built-in HTTPS** with Let's Encrypt (autocert)
- **SQLite + FTS5** for full-text search
- **Vector search** with in-memory brute-force index (OpenAI/Ollama embeddings)
- **MCP server** at `/mcp` with Bearer token auth
- **OAuth 2.0** with dynamic client registration + PKCE

## CLI

```
mykb serve stdio          # MCP over stdio (local)
mykb serve http           # HTTP server (config-driven)
mykb set-password         # Set auth password
mykb reindex [--force]    # Generate embeddings for chunks
```

Options:
- `--config PATH` - Config file (default: `~/.config/mykb/config.toml`)

## Configuration

Config file (`~/.config/mykb/config.toml`):

```toml
data_dir = "/var/lib/mykb"  # default: ~/.local/share/mykb

[server]
listen = ":8080"            # HTTP on localhost (dev)
# domain = "mykb.example.com" # HTTPS with auto TLS (prod, mutually exclusive with listen)
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

## Deployment

### Deploy changes

```bash
./deploy.sh
```

Requires `DEPLOY_HOST` in `.env` (e.g., `root@server`).

### Manual operations

```bash
# View logs
ssh $DEPLOY_HOST "journalctl -u mykb -f"

# Restart
ssh $DEPLOY_HOST "systemctl restart mykb"

# Status
ssh $DEPLOY_HOST "systemctl status mykb"

# Set password (first time)
ssh $DEPLOY_HOST "mykb set-password"
```

### Server configuration

Create config at `/etc/mykb/config.toml` or use `--config` flag.

Data stored in configured `data_dir` (database, TLS certs).

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point |
| `config/config.go` | Configuration loading (TOML) |
| `mcp/server.go` | MCP protocol handler (stdio + streamable HTTP) |
| `mcp/tools.go` | MCP tool definitions and handlers |
| `httpd/server.go` | HTTP server with autocert |
| `httpd/oauth.go` | OAuth endpoints (register, authorize, token) |
| `httpd/mcp.go` | MCP-over-HTTP transport |
| `storage/db.go` | SQLite schema and migrations |
| `storage/chunks.go` | Chunk CRUD + FTS5 search |
| `storage/embeddings.go` | Embedding storage |
| `storage/tokens.go` | OAuth token storage |
| `embedding/provider.go` | Embedding provider interface + config types |
| `embedding/openai.go` | OpenAI embedding provider |
| `embedding/ollama.go` | Ollama embedding provider |
| `vector/index.go` | In-memory vector index (brute-force) |

## OAuth Flow

Uses dynamic client registration (RFC 7591) + Authorization Code + PKCE:

1. Client discovers `/.well-known/oauth-authorization-server`
2. Client registers at `/register` → gets `client_id`
3. Client redirects to `/authorize` with PKCE challenge
4. User enters password, approves
5. Client exchanges code at `/token` → gets access token
6. Client uses Bearer token for MCP requests

## MCP Tools

- `store_chunk(content, metadata?)` - Store text with optional metadata (auto-generates embedding)
- `search_chunks(query, limit?)` - Full-text search with FTS5
- `semantic_search(query, limit?)` - Vector similarity search (requires embedding provider)
- `get_chunk(chunk_id)` - Get by ID
- `update_chunk(chunk_id, content?, metadata?)` - Update existing (re-generates embedding if content changed)
- `delete_chunk(chunk_id)` - Delete by ID
- `get_metadata_index(top_n?)` - Overview of metadata keys and values
- `get_metadata_values(key, top_n?)` - Drill down into specific metadata key

## Testing

```bash
go test ./...
```

Tests use in-memory SQLite.
