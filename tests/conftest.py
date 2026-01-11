"""Shared test fixtures."""

from collections.abc import AsyncGenerator

import aiosqlite
import fakeredis
import pytest

from app.database import init_db


@pytest.fixture
async def test_db() -> AsyncGenerator[aiosqlite.Connection, None]:
    """In-memory SQLite database with schema and migrations applied."""
    db = await aiosqlite.connect(":memory:")
    db.row_factory = aiosqlite.Row
    await init_db(db)
    yield db
    await db.close()


@pytest.fixture
async def fake_redis():
    """Fake Redis instance for testing."""
    async with fakeredis.FakeAsyncRedis(decode_responses=True) as client:
        yield client


@pytest.fixture
def patch_db(test_db, mocker):
    """Patch get_db to return test database."""

    async def mock_get_db():
        return test_db

    mocker.patch("app.database.get_db", mock_get_db)
    mocker.patch("app.chunks.get_db", mock_get_db)
    mocker.patch("app.oauth.routes.get_db", mock_get_db)


@pytest.fixture
def patch_redis(fake_redis, mocker):
    """Patch get_redis to return fake redis."""

    async def mock_get_redis():
        return fake_redis

    mocker.patch("app.oauth.tokens.get_redis", mock_get_redis)


@pytest.fixture
async def auth_token(fake_redis) -> str:
    """Create a valid access token in fake redis."""
    import json

    from app.oauth.tokens import generate_token, hash_token

    token = generate_token()
    token_hash = hash_token(token)
    await fake_redis.setex(
        f"access_token:{token_hash}",
        3600,
        json.dumps({"client_id": "test-client"}),
    )
    return token


@pytest.fixture
def auth_headers(auth_token) -> dict[str, str]:
    """Authorization headers with valid Bearer token."""
    return {"Authorization": f"Bearer {auth_token}"}
