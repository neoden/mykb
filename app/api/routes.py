from fastapi import APIRouter, HTTPException, Query

from app import chunks
from app.models import Chunk, ChunkCreate, ChunkList, ChunkUpdate, SearchResults

router = APIRouter(prefix="/api", tags=["chunks"])


@router.post("/chunks", response_model=Chunk, status_code=201)
async def create_chunk(data: ChunkCreate):
    """Create a new text chunk."""
    return await chunks.create_chunk(data)


@router.get("/chunks", response_model=ChunkList)
async def list_chunks(
    offset: int = Query(0, ge=0),
    limit: int = Query(50, ge=1, le=100),
):
    """List all chunks with pagination."""
    chunk_list, total = await chunks.list_chunks(offset, limit)
    return ChunkList(chunks=chunk_list, total=total, offset=offset, limit=limit)


@router.get("/chunks/{chunk_id}", response_model=Chunk)
async def get_chunk(chunk_id: str):
    """Get a specific chunk by ID."""
    chunk = await chunks.get_chunk(chunk_id)
    if not chunk:
        raise HTTPException(status_code=404, detail="Chunk not found")
    return chunk


@router.put("/chunks/{chunk_id}", response_model=Chunk)
async def update_chunk(chunk_id: str, data: ChunkUpdate):
    """Update a chunk."""
    chunk = await chunks.update_chunk(chunk_id, data)
    if not chunk:
        raise HTTPException(status_code=404, detail="Chunk not found")
    return chunk


@router.delete("/chunks/{chunk_id}", status_code=204)
async def delete_chunk(chunk_id: str):
    """Delete a chunk."""
    deleted = await chunks.delete_chunk(chunk_id)
    if not deleted:
        raise HTTPException(status_code=404, detail="Chunk not found")


@router.get("/search", response_model=SearchResults)
async def search(
    q: str = Query(..., min_length=1),
    limit: int = Query(20, ge=1, le=100),
):
    """Full-text search across chunks."""
    results = await chunks.search_chunks(q, limit)
    return SearchResults(results=results, query=q, total=len(results))
