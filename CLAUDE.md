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
- **SQLite + FTS5** for everything: chunks, OAuth tokens, clients, settings
- **MCP server** at `/mcp` with Bearer token auth
- **OAuth 2.0** with dynamic client registration + PKCE

## CLI

```
mykb serve stdio                  # MCP over stdio (local)
mykb serve http --listen :8080    # HTTP on localhost (dev)
mykb serve http --domain DOMAIN   # HTTPS with auto TLS (prod)
mykb set-password                 # Set auth password
```

Options:
- `--data DIR` - Data directory (default: `~/.local/share/mykb`, env: `MYKB_DATA`)
- `--behind-proxy` - Trust X-Forwarded-For header (env: `MYKB_BEHIND_PROXY=1`)

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
ssh $DEPLOY_HOST "mykb --data /var/lib/mykb set-password"
```

### Server configuration

On the server, create `/etc/mykb.env`:
```
MYKB_DOMAIN=mykb.example.com
```

Data stored in `/var/lib/mykb/` (database, TLS certs).

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entry point |
| `mcp/server.go` | MCP protocol handler (stdio + streamable HTTP) |
| `mcp/tools.go` | MCP tool definitions and handlers |
| `httpd/server.go` | HTTP server with autocert |
| `httpd/oauth.go` | OAuth endpoints (register, authorize, token) |
| `httpd/mcp.go` | MCP-over-HTTP transport |
| `storage/db.go` | SQLite schema and migrations |
| `storage/chunks.go` | Chunk CRUD + FTS5 search |
| `storage/tokens.go` | OAuth token storage |

## OAuth Flow

Uses dynamic client registration (RFC 7591) + Authorization Code + PKCE:

1. Client discovers `/.well-known/oauth-authorization-server`
2. Client registers at `/register` → gets `client_id`
3. Client redirects to `/authorize` with PKCE challenge
4. User enters password, approves
5. Client exchanges code at `/token` → gets access token
6. Client uses Bearer token for MCP requests

## MCP Tools

- `store_chunk(content, metadata?)` - Store text with optional metadata
- `search_chunks(query, limit?)` - Full-text search with FTS5
- `get_chunk(chunk_id)` - Get by ID
- `update_chunk(chunk_id, content?, metadata?)` - Update existing
- `delete_chunk(chunk_id)` - Delete by ID
- `get_metadata_index(top_n?)` - Overview of metadata keys and values
- `get_metadata_values(key, top_n?)` - Drill down into specific metadata key

## Testing

```bash
go test ./...
```

Tests use in-memory SQLite.
