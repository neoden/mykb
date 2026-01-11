from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI
from slowapi import _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded

from app.api.routes import router as api_router
from app.database import get_db, init_db
from app.mcp.server import mcp
from app.oauth.middleware import require_auth
from app.oauth.routes import limiter, router as oauth_router

# Get MCP http app with its lifespan
mcp_app = mcp.http_app()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize database and MCP on startup."""
    db = await get_db()
    await init_db(db)
    # Run MCP's lifespan
    async with mcp_app.lifespan(mcp_app):
        yield


app = FastAPI(
    title="MyKB",
    description="Text chunk storage and search service with MCP interface",
    version="0.1.0",
    lifespan=lifespan,
)

# Rate limiting
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)

# OAuth endpoints (public)
app.include_router(oauth_router)

# REST API endpoints (protected)
app.include_router(api_router, dependencies=[Depends(require_auth)])

# Mount MCP server (auth handled by FastMCP via RedisTokenVerifier)
# FastMCP http_app() already has /mcp as its route, so mount at root
app.mount("/", mcp_app)


@app.get("/health")
async def health():
    """Health check endpoint."""
    return {"status": "ok"}


if __name__ == "__main__":
    import uvicorn

    from app.config import settings

    uvicorn.run(app, host=settings.host, port=settings.port)
