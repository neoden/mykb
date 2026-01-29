from datetime import datetime
from pydantic import BaseModel, Field


# Chunk models
class ChunkCreate(BaseModel):
    content: str
    metadata: dict | None = None


class ChunkUpdate(BaseModel):
    content: str | None = None
    metadata: dict | None = None


class Chunk(BaseModel):
    id: str
    content: str
    metadata: dict | None = None
    created_at: datetime
    updated_at: datetime


class ChunkList(BaseModel):
    chunks: list[Chunk]
    total: int
    offset: int
    limit: int


class SearchResult(BaseModel):
    id: str
    content: str
    metadata: dict | None = None
    snippet: str  # highlighted match


class SearchResults(BaseModel):
    results: list[SearchResult]
    query: str
    total: int


# OAuth models
class ClientRegistration(BaseModel):
    client_name: str | None = None
    redirect_uris: list[str]


class ClientRegistrationResponse(BaseModel):
    client_id: str
    client_name: str | None = None
    redirect_uris: list[str]


class TokenRequest(BaseModel):
    grant_type: str
    code: str | None = None
    redirect_uri: str | None = None
    code_verifier: str | None = None
    client_id: str | None = None


class TokenResponse(BaseModel):
    access_token: str
    token_type: str = "Bearer"
    expires_in: int
    refresh_token: str | None = None


class OAuthMetadata(BaseModel):
    issuer: str
    authorization_endpoint: str
    token_endpoint: str
    registration_endpoint: str
    response_types_supported: list[str] = ["code"]
    grant_types_supported: list[str] = ["authorization_code"]
    code_challenge_methods_supported: list[str] = ["S256"]


class ProtectedResourceMetadata(BaseModel):
    """OAuth 2.0 Protected Resource Metadata (RFC 9728)."""
    resource: str
    authorization_servers: list[str]
