"""Tests for MCP tools."""

import pytest
from fastmcp.client import Client

from app.mcp.server import mcp

pytestmark = pytest.mark.usefixtures("patch_db", "patch_redis")


@pytest.fixture
async def mcp_client():
    """MCP client for testing tools."""
    async with Client(transport=mcp) as client:
        yield client


@pytest.mark.asyncio
async def test_store_chunk(mcp_client):
    """Test store_chunk tool."""
    result = await mcp_client.call_tool(
        "store_chunk", {"content": "Hello MCP", "metadata": {"source": "test"}}
    )

    assert "id" in result.data
    assert result.data["content"] == "Hello MCP"
    assert result.data["metadata"] == {"source": "test"}


@pytest.mark.asyncio
async def test_store_chunk_without_metadata(mcp_client):
    """Test store_chunk tool without metadata."""
    result = await mcp_client.call_tool("store_chunk", {"content": "No metadata"})

    assert result.data["content"] == "No metadata"
    assert result.data["metadata"] is None


@pytest.mark.asyncio
async def test_get_chunk(mcp_client):
    """Test get_chunk tool."""
    created = await mcp_client.call_tool("store_chunk", {"content": "Test content"})
    chunk_id = created.data["id"]

    result = await mcp_client.call_tool("get_chunk", {"chunk_id": chunk_id})

    assert result.data is not None
    assert result.data["id"] == chunk_id
    assert result.data["content"] == "Test content"


@pytest.mark.asyncio
async def test_get_chunk_not_found(mcp_client):
    """Test get_chunk tool with non-existent ID."""
    result = await mcp_client.call_tool("get_chunk", {"chunk_id": "non-existent"})
    assert result.data is None


@pytest.mark.asyncio
async def test_list_chunks(mcp_client):
    """Test list_chunks tool with pagination."""
    for i in range(5):
        await mcp_client.call_tool("store_chunk", {"content": f"Chunk {i}"})

    result = await mcp_client.call_tool("list_chunks", {"offset": 0, "limit": 3})

    assert result.data["total"] == 5
    assert len(result.data["chunks"]) == 3
    assert result.data["offset"] == 0
    assert result.data["limit"] == 3


@pytest.mark.asyncio
async def test_list_chunks_limit_capped(mcp_client):
    """Test that list_chunks caps limit at 100."""
    result = await mcp_client.call_tool("list_chunks", {"limit": 200})
    assert result.data["limit"] == 100


@pytest.mark.asyncio
async def test_update_chunk(mcp_client):
    """Test update_chunk tool."""
    created = await mcp_client.call_tool(
        "store_chunk", {"content": "Original", "metadata": {"v": 1}}
    )
    chunk_id = created.data["id"]

    result = await mcp_client.call_tool(
        "update_chunk", {"chunk_id": chunk_id, "content": "Updated", "metadata": {"v": 2}}
    )

    assert result.data is not None
    assert result.data["content"] == "Updated"
    assert result.data["metadata"] == {"v": 2}


@pytest.mark.asyncio
async def test_update_chunk_partial(mcp_client):
    """Test partial update with update_chunk tool."""
    created = await mcp_client.call_tool(
        "store_chunk", {"content": "Original", "metadata": {"keep": "this"}}
    )
    chunk_id = created.data["id"]

    result = await mcp_client.call_tool(
        "update_chunk", {"chunk_id": chunk_id, "content": "New content"}
    )

    assert result.data["content"] == "New content"
    assert result.data["metadata"] == {"keep": "this"}


@pytest.mark.asyncio
async def test_update_chunk_not_found(mcp_client):
    """Test update_chunk tool with non-existent ID."""
    result = await mcp_client.call_tool(
        "update_chunk", {"chunk_id": "non-existent", "content": "Updated"}
    )
    assert result.data is None


@pytest.mark.asyncio
async def test_delete_chunk(mcp_client):
    """Test delete_chunk tool."""
    created = await mcp_client.call_tool("store_chunk", {"content": "To delete"})
    chunk_id = created.data["id"]

    result = await mcp_client.call_tool("delete_chunk", {"chunk_id": chunk_id})
    assert result.data is True

    # Verify it's gone
    fetched = await mcp_client.call_tool("get_chunk", {"chunk_id": chunk_id})
    assert fetched.data is None


@pytest.mark.asyncio
async def test_delete_chunk_not_found(mcp_client):
    """Test delete_chunk tool with non-existent ID."""
    result = await mcp_client.call_tool("delete_chunk", {"chunk_id": "non-existent"})
    assert result.data is False


@pytest.mark.asyncio
async def test_search_chunks(mcp_client):
    """Test search_chunks tool."""
    await mcp_client.call_tool("store_chunk", {"content": "Python programming language"})
    await mcp_client.call_tool("store_chunk", {"content": "JavaScript for web"})
    await mcp_client.call_tool("store_chunk", {"content": "Python web framework"})

    result = await mcp_client.call_tool("search_chunks", {"query": "Python"})

    assert len(result.data) == 2
    # structured_content has {"result": [...]} wrapper
    items = result.structured_content.get("result", result.structured_content)
    assert all("Python" in item["content"] for item in items)


@pytest.mark.asyncio
async def test_search_chunks_with_limit(mcp_client):
    """Test search_chunks tool with limit."""
    for i in range(10):
        await mcp_client.call_tool("store_chunk", {"content": f"Document number {i}"})

    result = await mcp_client.call_tool("search_chunks", {"query": "Document", "limit": 3})

    assert len(result.data) == 3


@pytest.mark.asyncio
async def test_search_chunks_no_results(mcp_client):
    """Test search_chunks tool with no matches."""
    await mcp_client.call_tool("store_chunk", {"content": "Hello world"})

    result = await mcp_client.call_tool("search_chunks", {"query": "nonexistent"})

    assert result.data == []


@pytest.mark.asyncio
async def test_get_metadata_index(mcp_client):
    """Test get_metadata_index tool."""
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Book one", "metadata": {"tags": ["book", "fiction"], "author": "Alice"}},
    )
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Book two", "metadata": {"tags": ["book", "science"], "author": "Bob"}},
    )
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "No metadata"},
    )

    result = await mcp_client.call_tool("get_metadata_index", {})

    assert result.data["total_chunks"] == 3
    keys = result.data["keys"]
    assert "tags" in keys
    assert "author" in keys
    # "book" tag appears in 2 chunks
    assert keys["tags"]["book"] == 2
    assert keys["tags"]["fiction"] == 1
    assert keys["tags"]["science"] == 1


@pytest.mark.asyncio
async def test_get_metadata_index_empty(mcp_client):
    """Test get_metadata_index with no chunks."""
    result = await mcp_client.call_tool("get_metadata_index", {})

    assert result.data["total_chunks"] == 0
    assert result.data["keys"] == {}


@pytest.mark.asyncio
async def test_get_metadata_values(mcp_client):
    """Test get_metadata_values for drilling down into a specific key."""
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Book one", "metadata": {"tags": ["python", "async"]}},
    )
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Book two", "metadata": {"tags": ["python", "web"]}},
    )
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Book three", "metadata": {"tags": ["rust"]}},
    )

    result = await mcp_client.call_tool("get_metadata_values", {"key": "tags"})

    assert result.data["key"] == "tags"
    values = result.data["values"]
    assert values["python"] == 2
    assert values["async"] == 1
    assert values["web"] == 1
    assert values["rust"] == 1


@pytest.mark.asyncio
async def test_get_metadata_values_nonexistent_key(mcp_client):
    """Test get_metadata_values for a key that doesn't exist."""
    await mcp_client.call_tool(
        "store_chunk",
        {"content": "Test", "metadata": {"source": "docs"}},
    )

    result = await mcp_client.call_tool("get_metadata_values", {"key": "nonexistent"})

    assert result.data["key"] == "nonexistent"
    assert result.data["values"] == {}
