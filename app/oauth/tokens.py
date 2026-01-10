import hashlib
import json
import secrets
from base64 import urlsafe_b64encode

from fastmcp.server.auth import AccessToken, TokenVerifier
from redis.asyncio import Redis

from app.config import settings

_redis: Redis | None = None


async def get_redis() -> Redis:
    """Get Redis connection."""
    global _redis
    if _redis is None:
        _redis = Redis.from_url(settings.redis_url, decode_responses=True)
    return _redis


def generate_token() -> str:
    """Generate a secure random token."""
    return urlsafe_b64encode(secrets.token_bytes(32)).decode("utf-8").rstrip("=")


def hash_token(token: str) -> str:
    """Hash a token for storage."""
    return hashlib.sha256(token.encode()).hexdigest()


def hash_pkce_verifier(verifier: str) -> str:
    """Create S256 hash of PKCE code verifier."""
    digest = hashlib.sha256(verifier.encode()).digest()
    return urlsafe_b64encode(digest).decode("utf-8").rstrip("=")


async def store_auth_code(
    code: str,
    client_id: str,
    redirect_uri: str,
    code_challenge: str,
    code_challenge_method: str = "S256",
) -> None:
    """Store authorization code in Redis with TTL."""
    redis = await get_redis()
    code_hash = hash_token(code)
    data = json.dumps(
        {
            "client_id": client_id,
            "redirect_uri": redirect_uri,
            "code_challenge": code_challenge,
            "code_challenge_method": code_challenge_method,
        }
    )
    await redis.setex(f"auth_code:{code_hash}", settings.code_expiry_seconds, data)


async def get_auth_code(code: str) -> dict | None:
    """Get and delete authorization code from Redis."""
    redis = await get_redis()
    code_hash = hash_token(code)
    key = f"auth_code:{code_hash}"
    data = await redis.get(key)
    if data:
        await redis.delete(key)  # One-time use
        return json.loads(data)
    return None


async def store_access_token(token: str, client_id: str) -> None:
    """Store access token in Redis with TTL."""
    redis = await get_redis()
    token_hash = hash_token(token)
    data = json.dumps({"client_id": client_id})
    await redis.setex(f"access_token:{token_hash}", settings.token_expiry_seconds, data)


async def validate_access_token(token: str) -> dict | None:
    """Validate access token and return associated data."""
    redis = await get_redis()
    token_hash = hash_token(token)
    data = await redis.get(f"access_token:{token_hash}")
    if data:
        return json.loads(data)
    return None


async def store_refresh_token(token: str, client_id: str) -> None:
    """Store refresh token in Redis with longer TTL."""
    redis = await get_redis()
    token_hash = hash_token(token)
    data = json.dumps({"client_id": client_id})
    await redis.setex(
        f"refresh_token:{token_hash}", settings.refresh_token_expiry_seconds, data
    )


async def validate_refresh_token(token: str) -> dict | None:
    """Validate refresh token and return associated data."""
    redis = await get_redis()
    token_hash = hash_token(token)
    data = await redis.get(f"refresh_token:{token_hash}")
    if data:
        return json.loads(data)
    return None


async def revoke_refresh_token(token: str) -> None:
    """Revoke a refresh token."""
    redis = await get_redis()
    token_hash = hash_token(token)
    await redis.delete(f"refresh_token:{token_hash}")


class RedisTokenVerifier(TokenVerifier):
    """Custom token verifier that validates tokens against Redis."""

    async def verify_token(self, token: str) -> AccessToken | None:
        """Verify a bearer token and return access info if valid."""
        data = await validate_access_token(token)
        if data:
            return AccessToken(
                token=token,
                client_id=data.get("client_id", ""),
                scopes=[],
                claims=data,
            )
        return None
