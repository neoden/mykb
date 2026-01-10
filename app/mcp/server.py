from fastmcp import FastMCP

from app import chunks
from app.config import settings
from app.models import ChunkCreate, ChunkUpdate
from app.oauth.tokens import RedisTokenVerifier

# Create MCP server with auth
auth = RedisTokenVerifier(base_url=settings.base_url)
mcp = FastMCP("MyKB", instructions="Text chunk storage and search service", auth=auth)


@mcp.tool()
async def store_chunk(content: str, metadata: dict | None = None) -> dict:
    """Store a new text chunk with optional metadata.

    Args:
        content: The text content to store
        metadata: Optional metadata dict (tags, source, etc.)

    Returns:
        The created chunk with ID
    """
    data = ChunkCreate(content=content, metadata=metadata)
    chunk = await chunks.create_chunk(data)
    return chunk.model_dump(mode="json")


@mcp.tool()
async def search_chunks(query: str, limit: int = 20) -> list[dict]:
    """Full-text search across all stored chunks.

    Args:
        query: Search query (supports FTS5 syntax)
        limit: Maximum results to return (default 20)

    Returns:
        List of matching chunks with highlighted snippets
    """
    results = await chunks.search_chunks(query, limit)
    return [r.model_dump(mode="json") for r in results]


@mcp.tool()
async def get_chunk(chunk_id: str) -> dict | None:
    """Get a specific chunk by ID.

    Args:
        chunk_id: The UUID of the chunk

    Returns:
        The chunk if found, None otherwise
    """
    chunk = await chunks.get_chunk(chunk_id)
    if chunk:
        return chunk.model_dump(mode="json")
    return None


@mcp.tool()
async def list_chunks(offset: int = 0, limit: int = 50) -> dict:
    """List all stored chunks with pagination.

    Args:
        offset: Number of chunks to skip
        limit: Maximum chunks to return (default 50, max 100)

    Returns:
        Dict with chunks list and pagination info
    """
    limit = min(limit, 100)
    chunk_list, total = await chunks.list_chunks(offset, limit)
    return {
        "chunks": [c.model_dump(mode="json") for c in chunk_list],
        "total": total,
        "offset": offset,
        "limit": limit,
    }


@mcp.tool()
async def update_chunk(chunk_id: str, content: str | None = None, metadata: dict | None = None) -> dict | None:
    """Update an existing chunk.

    Args:
        chunk_id: The UUID of the chunk to update
        content: New content (optional)
        metadata: New metadata (optional)

    Returns:
        The updated chunk if found, None otherwise
    """
    data = ChunkUpdate(content=content, metadata=metadata)
    chunk = await chunks.update_chunk(chunk_id, data)
    if chunk:
        return chunk.model_dump(mode="json")
    return None


@mcp.tool()
async def delete_chunk(chunk_id: str) -> bool:
    """Delete a chunk by ID.

    Args:
        chunk_id: The UUID of the chunk to delete

    Returns:
        True if deleted, False if not found
    """
    return await chunks.delete_chunk(chunk_id)
