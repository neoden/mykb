# MyKB - Project Guide

## What is this?

A self-hosted text chunk storage and search service exposed via MCP (Model Context Protocol). It allows Claude to store and retrieve knowledge chunks with full-text search.

## Architecture

```
Internet → Caddy (:443) → FastAPI (:8000) → SQLite + Redis
                              ↓
                         FastMCP (/mcp)
```

- **FastAPI** - Main app, handles OAuth and REST API
- **FastMCP** - MCP server mounted at `/mcp` with Bearer token auth
- **SQLite + FTS5** - Persistent storage with full-text search
- **Redis** - Ephemeral OAuth tokens and auth codes (TTL-based)
- **Caddy** - Reverse proxy with auto HTTPS

## Deployment

Configuration is in `.env` (see `.env.example`). MCP URL is `${BASE_URL}/mcp`.

### Deploy changes

```bash
./deploy.sh
```

### Manual operations

Read `DEPLOY_HOST` from `.env` for SSH commands:

```bash
# View logs
ssh $DEPLOY_HOST "cd /opt/mykb && docker compose logs -f app"

# Restart all services
ssh $DEPLOY_HOST "cd /opt/mykb && docker compose restart"

# Check status
ssh $DEPLOY_HOST "cd /opt/mykb && docker compose ps"
```

### Configuration

Local `.env` file contains:
- `AUTH_PASSWORD` - Password for OAuth authorization screen
- `BASE_URL` - Public URL of the service
- `DEPLOY_HOST` - SSH target for deployment (user@host)

Copy `Caddyfile.example` to `Caddyfile` and set your domain.

## Key Files

| File | Purpose |
|------|---------|
| `app/main.py` | FastAPI app, mounts MCP server with lifespan |
| `app/mcp/server.py` | FastMCP server with tools (store, search, get, list, delete) |
| `app/oauth/routes.py` | OAuth endpoints (register, authorize, token) |
| `app/oauth/tokens.py` | Redis token storage + `RedisTokenVerifier` for MCP auth |
| `app/chunks.py` | CRUD operations for chunks (SQLite) |
| `app/database.py` | SQLite schema with FTS5 |

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
- `list_chunks(offset?, limit?)` - Paginated list
- `update_chunk(chunk_id, content?, metadata?)` - Update existing
- `delete_chunk(chunk_id)` - Delete by ID

## REST API

Protected by Bearer token (same as MCP):

- `POST /api/chunks` - Create
- `GET /api/chunks` - List
- `GET /api/chunks/{id}` - Get
- `PUT /api/chunks/{id}` - Update
- `DELETE /api/chunks/{id}` - Delete
- `GET /api/search?q=...` - Search

## Testing

```bash
uv run pytest
```

Tests use in-memory SQLite and fakeredis. Fixtures in `tests/conftest.py` patch `get_db()` and `get_redis()`.

| File | Coverage |
|------|----------|
| `tests/test_api.py` | REST API endpoints |
| `tests/test_mcp.py` | MCP tools via FastMCP Client |
| `tests/test_oauth.py` | OAuth flow + token utilities |
