import json
import uuid
from datetime import datetime

import aiosqlite

from app.database import get_db
from app.models import Chunk, ChunkCreate, ChunkUpdate, SearchResult


async def create_chunk(data: ChunkCreate) -> Chunk:
    """Create a new chunk."""
    chunk_id = str(uuid.uuid4())
    now = datetime.utcnow()
    metadata_json = json.dumps(data.metadata) if data.metadata else None

    db = await get_db()
    await db.execute(
        """
        INSERT INTO chunks (id, content, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?)
        """,
        (chunk_id, data.content, metadata_json, now, now),
    )
    await db.commit()

    return Chunk(
        id=chunk_id,
        content=data.content,
        metadata=data.metadata,
        created_at=now,
        updated_at=now,
    )


async def get_chunk(chunk_id: str) -> Chunk | None:
    """Get a chunk by ID."""
    db = await get_db()
    cursor = await db.execute(
        "SELECT id, content, metadata, created_at, updated_at FROM chunks WHERE id = ?",
        (chunk_id,),
    )
    row = await cursor.fetchone()
    if not row:
        return None
    return Chunk(
        id=row["id"],
        content=row["content"],
        metadata=json.loads(row["metadata"]) if row["metadata"] else None,
        created_at=row["created_at"],
        updated_at=row["updated_at"],
    )


async def update_chunk(chunk_id: str, data: ChunkUpdate) -> Chunk | None:
    """Update a chunk."""
    existing = await get_chunk(chunk_id)
    if not existing:
        return None

    now = datetime.utcnow()
    content = data.content if data.content is not None else existing.content
    metadata = data.metadata if data.metadata is not None else existing.metadata
    metadata_json = json.dumps(metadata) if metadata else None

    db = await get_db()
    await db.execute(
        """
        UPDATE chunks SET content = ?, metadata = ?, updated_at = ?
        WHERE id = ?
        """,
        (content, metadata_json, now, chunk_id),
    )
    await db.commit()

    return Chunk(
        id=chunk_id,
        content=content,
        metadata=metadata,
        created_at=existing.created_at,
        updated_at=now,
    )


async def delete_chunk(chunk_id: str) -> bool:
    """Delete a chunk."""
    db = await get_db()
    cursor = await db.execute("DELETE FROM chunks WHERE id = ?", (chunk_id,))
    await db.commit()
    return cursor.rowcount > 0


async def list_chunks(offset: int = 0, limit: int = 50) -> tuple[list[Chunk], int]:
    """List chunks with pagination."""
    db = await get_db()

    # Get total count
    cursor = await db.execute("SELECT COUNT(*) as count FROM chunks")
    row = await cursor.fetchone()
    total = row["count"]

    # Get paginated results
    cursor = await db.execute(
        """
        SELECT id, content, metadata, created_at, updated_at
        FROM chunks ORDER BY created_at DESC LIMIT ? OFFSET ?
        """,
        (limit, offset),
    )
    rows = await cursor.fetchall()

    chunks = [
        Chunk(
            id=row["id"],
            content=row["content"],
            metadata=json.loads(row["metadata"]) if row["metadata"] else None,
            created_at=row["created_at"],
            updated_at=row["updated_at"],
        )
        for row in rows
    ]
    return chunks, total


async def search_chunks(query: str, limit: int = 20) -> list[SearchResult]:
    """Full-text search across chunks."""
    # Escape each token for FTS5: wrap individual words in quotes
    tokens = query.split()
    escaped_tokens = ['"' + t.replace('"', '""') + '"' for t in tokens]
    escaped_query = ' '.join(escaped_tokens)
    db = await get_db()
    cursor = await db.execute(
        """
        SELECT c.id, c.content, c.metadata, snippet(chunks_fts, 1, '<mark>', '</mark>', '...', 32) as snippet
        FROM chunks_fts fts
        JOIN chunks c ON fts.id = c.id
        WHERE chunks_fts MATCH ?
        ORDER BY rank
        LIMIT ?
        """,
        (escaped_query, limit),
    )
    rows = await cursor.fetchall()

    return [
        SearchResult(
            id=row["id"],
            content=row["content"],
            metadata=json.loads(row["metadata"]) if row["metadata"] else None,
            snippet=row["snippet"],
        )
        for row in rows
    ]
