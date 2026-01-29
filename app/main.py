import logging
import sys
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, Request
from fastapi.responses import JSONResponse
from slowapi import _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded

from app.api.routes import router as api_router
from app.database import get_db, init_db
from app.mcp.server import mcp
from app.oauth.middleware import require_auth
from app.oauth.routes import limiter, router as oauth_router

# Configure logging to stdout
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger(__name__)

# Get MCP http app (stateless to handle session-id issues with proxies)
mcp_app = mcp.http_app(stateless_http=True)


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


@app.exception_handler(Exception)
async def unhandled_exception_handler(request: Request, exc: Exception):
    """Log unhandled exceptions with traceback."""
    logger.exception(f"Unhandled exception on {request.method} {request.url.path}")
    return JSONResponse(status_code=500, content={"detail": "Internal server error"})


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
