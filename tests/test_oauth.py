"""Tests for OAuth endpoints and token functions."""

import pytest
from httpx import ASGITransport, AsyncClient

from app.main import app
from app.oauth.tokens import generate_token, hash_pkce_verifier, hash_token

pytestmark = pytest.mark.usefixtures("patch_db", "patch_redis")


@pytest.fixture
async def client():
    """Async HTTP client for testing."""
    async with AsyncClient(
        transport=ASGITransport(app=app),
        base_url="http://test",
    ) as client:
        yield client


# Token utility tests


def test_generate_token():
    """Test that generate_token produces unique tokens."""
    token1 = generate_token()
    token2 = generate_token()

    assert token1 != token2
    assert len(token1) > 20  # Should be reasonably long


def test_hash_token():
    """Test that hash_token is deterministic."""
    token = "test-token"

    hash1 = hash_token(token)
    hash2 = hash_token(token)

    assert hash1 == hash2
    assert hash1 != token  # Should be different from input


def test_hash_pkce_verifier():
    """Test PKCE S256 hash."""
    verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

    challenge = hash_pkce_verifier(verifier)

    # Should produce base64url-encoded SHA256 without padding
    assert "=" not in challenge
    assert len(challenge) == 43  # SHA256 = 32 bytes = 43 base64 chars


# OAuth endpoint tests


@pytest.mark.asyncio
async def test_oauth_metadata(client):
    """Test OAuth metadata endpoint."""
    response = await client.get("/.well-known/oauth-authorization-server")

    assert response.status_code == 200
    data = response.json()
    assert "authorization_endpoint" in data
    assert "token_endpoint" in data
    assert "registration_endpoint" in data
    assert "S256" in data["code_challenge_methods_supported"]


@pytest.mark.asyncio
async def test_register_client(client):
    """Test client registration."""
    response = await client.post(
        "/register",
        json={
            "client_name": "Test Client",
            "redirect_uris": ["http://localhost:8080/callback"],
        },
    )

    assert response.status_code == 200
    data = response.json()
    assert "client_id" in data
    assert data["client_name"] == "Test Client"
    assert data["redirect_uris"] == ["http://localhost:8080/callback"]


@pytest.mark.asyncio
async def test_authorize_get(client):
    """Test authorization form display."""
    # Register a client first
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    response = await client.get(
        "/authorize",
        params={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "response_type": "code",
            "code_challenge": "test-challenge",
            "code_challenge_method": "S256",
        },
    )

    assert response.status_code == 200
    assert "password" in response.text
    assert "csrf_token" in response.text


@pytest.mark.asyncio
async def test_authorize_invalid_client(client):
    """Test authorization with invalid client_id."""
    response = await client.get(
        "/authorize",
        params={
            "client_id": "invalid",
            "redirect_uri": "http://localhost/callback",
            "response_type": "code",
            "code_challenge": "test-challenge",
        },
    )

    assert response.status_code == 400


@pytest.mark.asyncio
async def test_authorize_invalid_redirect_uri(client):
    """Test authorization with non-matching redirect_uri."""
    # Register a client
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    response = await client.get(
        "/authorize",
        params={
            "client_id": client_id,
            "redirect_uri": "http://evil.com/callback",
            "response_type": "code",
            "code_challenge": "test-challenge",
        },
    )

    assert response.status_code == 400


@pytest.mark.asyncio
async def test_full_oauth_flow(client, mocker):
    """Test complete OAuth authorization code flow with PKCE."""
    # Mock settings for password
    mocker.patch("app.oauth.routes.settings.auth_password", "testpass")

    # 1. Register client
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    # 2. Generate PKCE verifier and challenge
    verifier = generate_token()
    challenge = hash_pkce_verifier(verifier)

    # 3. Get authorization form
    auth_get_resp = await client.get(
        "/authorize",
        params={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "response_type": "code",
            "code_challenge": challenge,
            "code_challenge_method": "S256",
            "state": "test-state",
        },
    )
    assert auth_get_resp.status_code == 200

    # Extract CSRF token from form
    import re

    csrf_match = re.search(r'name="csrf_token" value="([^"]+)"', auth_get_resp.text)
    assert csrf_match
    csrf_token = csrf_match.group(1)

    # 4. Submit authorization form
    auth_post_resp = await client.post(
        "/authorize",
        data={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "code_challenge": challenge,
            "code_challenge_method": "S256",
            "state": "test-state",
            "csrf_token": csrf_token,
            "password": "testpass",
        },
        follow_redirects=False,
    )
    assert auth_post_resp.status_code == 302

    # Extract code from redirect
    location = auth_post_resp.headers["location"]
    assert "code=" in location
    code_match = re.search(r"code=([^&]+)", location)
    assert code_match
    code = code_match.group(1)

    # 5. Exchange code for tokens
    token_resp = await client.post(
        "/token",
        data={
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": "http://localhost/callback",
            "code_verifier": verifier,
            "client_id": client_id,
        },
    )
    assert token_resp.status_code == 200
    tokens = token_resp.json()
    assert "access_token" in tokens
    assert "refresh_token" in tokens
    assert tokens["token_type"] == "Bearer"


@pytest.mark.asyncio
async def test_token_refresh(client, mocker):
    """Test refresh token flow."""
    mocker.patch("app.oauth.routes.settings.auth_password", "testpass")

    # Go through auth flow to get tokens
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    verifier = generate_token()
    challenge = hash_pkce_verifier(verifier)

    auth_get_resp = await client.get(
        "/authorize",
        params={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "response_type": "code",
            "code_challenge": challenge,
        },
    )

    import re

    csrf_match = re.search(r'name="csrf_token" value="([^"]+)"', auth_get_resp.text)
    csrf_token = csrf_match.group(1)

    auth_post_resp = await client.post(
        "/authorize",
        data={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "code_challenge": challenge,
            "csrf_token": csrf_token,
            "password": "testpass",
        },
        follow_redirects=False,
    )

    code_match = re.search(r"code=([^&]+)", auth_post_resp.headers["location"])
    code = code_match.group(1)

    token_resp = await client.post(
        "/token",
        data={
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": "http://localhost/callback",
            "code_verifier": verifier,
            "client_id": client_id,
        },
    )
    refresh_token = token_resp.json()["refresh_token"]

    # Now test refresh
    refresh_resp = await client.post(
        "/token",
        data={
            "grant_type": "refresh_token",
            "refresh_token": refresh_token,
        },
    )

    assert refresh_resp.status_code == 200
    new_tokens = refresh_resp.json()
    assert "access_token" in new_tokens
    assert "refresh_token" in new_tokens
    # New refresh token should be different (rotation)
    assert new_tokens["refresh_token"] != refresh_token


@pytest.mark.asyncio
async def test_invalid_csrf_token(client):
    """Test that invalid CSRF token is rejected."""
    # Register client
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    response = await client.post(
        "/authorize",
        data={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "code_challenge": "test",
            "csrf_token": "invalid-csrf",
            "password": "anything",
        },
    )

    assert response.status_code == 400
    assert "CSRF" in response.json()["detail"]


@pytest.mark.asyncio
async def test_invalid_password(client, mocker):
    """Test that wrong password is rejected."""
    mocker.patch("app.oauth.routes.settings.auth_password", "correct")

    # Register client
    reg_resp = await client.post(
        "/register",
        json={"redirect_uris": ["http://localhost/callback"]},
    )
    client_id = reg_resp.json()["client_id"]

    # Get CSRF token
    auth_get_resp = await client.get(
        "/authorize",
        params={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "response_type": "code",
            "code_challenge": "test",
        },
    )

    import re

    csrf_match = re.search(r'name="csrf_token" value="([^"]+)"', auth_get_resp.text)
    csrf_token = csrf_match.group(1)

    response = await client.post(
        "/authorize",
        data={
            "client_id": client_id,
            "redirect_uri": "http://localhost/callback",
            "code_challenge": "test",
            "csrf_token": csrf_token,
            "password": "wrong",
        },
    )

    assert response.status_code == 401
