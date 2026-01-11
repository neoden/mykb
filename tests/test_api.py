"""Tests for REST API endpoints."""

import pytest
from httpx import ASGITransport, AsyncClient

from app.main import app

pytestmark = pytest.mark.usefixtures("patch_db", "patch_redis")


@pytest.fixture
async def client():
    """Async HTTP client for testing."""
    async with AsyncClient(
        transport=ASGITransport(app=app),
        base_url="http://test",
    ) as client:
        yield client


@pytest.mark.asyncio
async def test_create_chunk(client, auth_headers):
    """Test POST /api/chunks."""
    response = await client.post(
        "/api/chunks",
        json={"content": "Hello world", "metadata": {"tag": "test"}},
        headers=auth_headers,
    )

    assert response.status_code == 201
    data = response.json()
    assert data["content"] == "Hello world"
    assert data["metadata"] == {"tag": "test"}
    assert "id" in data


@pytest.mark.asyncio
async def test_get_chunk(client, auth_headers):
    """Test GET /api/chunks/{id}."""
    # Create a chunk first
    create_resp = await client.post(
        "/api/chunks",
        json={"content": "Test content"},
        headers=auth_headers,
    )
    chunk_id = create_resp.json()["id"]

    # Get it
    response = await client.get(f"/api/chunks/{chunk_id}", headers=auth_headers)

    assert response.status_code == 200
    assert response.json()["content"] == "Test content"


@pytest.mark.asyncio
async def test_get_chunk_not_found(client, auth_headers):
    """Test GET /api/chunks/{id} with non-existent ID."""
    response = await client.get("/api/chunks/non-existent", headers=auth_headers)
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_list_chunks(client, auth_headers):
    """Test GET /api/chunks with pagination."""
    # Create some chunks
    for i in range(5):
        await client.post(
            "/api/chunks",
            json={"content": f"Chunk {i}"},
            headers=auth_headers,
        )

    response = await client.get(
        "/api/chunks",
        params={"limit": 3},
        headers=auth_headers,
    )

    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 5
    assert len(data["chunks"]) == 3


@pytest.mark.asyncio
async def test_update_chunk(client, auth_headers):
    """Test PUT /api/chunks/{id}."""
    # Create a chunk
    create_resp = await client.post(
        "/api/chunks",
        json={"content": "Original"},
        headers=auth_headers,
    )
    chunk_id = create_resp.json()["id"]

    # Update it
    response = await client.put(
        f"/api/chunks/{chunk_id}",
        json={"content": "Updated"},
        headers=auth_headers,
    )

    assert response.status_code == 200
    assert response.json()["content"] == "Updated"


@pytest.mark.asyncio
async def test_delete_chunk(client, auth_headers):
    """Test DELETE /api/chunks/{id}."""
    # Create a chunk
    create_resp = await client.post(
        "/api/chunks",
        json={"content": "To delete"},
        headers=auth_headers,
    )
    chunk_id = create_resp.json()["id"]

    # Delete it
    response = await client.delete(f"/api/chunks/{chunk_id}", headers=auth_headers)
    assert response.status_code == 204

    # Verify it's gone
    get_resp = await client.get(f"/api/chunks/{chunk_id}", headers=auth_headers)
    assert get_resp.status_code == 404


@pytest.mark.asyncio
async def test_search_chunks(client, auth_headers):
    """Test GET /api/search."""
    # Create chunks
    await client.post(
        "/api/chunks",
        json={"content": "Python programming"},
        headers=auth_headers,
    )
    await client.post(
        "/api/chunks",
        json={"content": "JavaScript web"},
        headers=auth_headers,
    )

    response = await client.get(
        "/api/search",
        params={"q": "Python"},
        headers=auth_headers,
    )

    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) == 1
    assert "Python" in data["results"][0]["content"]


@pytest.mark.asyncio
async def test_unauthorized_without_token(client):
    """Test that API requires authentication."""
    response = await client.get("/api/chunks")
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_unauthorized_invalid_token(client):
    """Test that invalid tokens are rejected."""
    response = await client.get(
        "/api/chunks",
        headers={"Authorization": "Bearer invalid-token"},
    )
    assert response.status_code == 401
