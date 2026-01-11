import hmac
import html
import json
import uuid
from urllib.parse import urlencode

from fastapi import APIRouter, Form, HTTPException, Query, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from slowapi import Limiter
from slowapi.util import get_remote_address

from app.config import settings
from app.database import delete_stale_clients, get_db, touch_client
from app.models import (
    ClientRegistration,
    ClientRegistrationResponse,
    OAuthMetadata,
    TokenResponse,
)
from app.oauth.tokens import (
    generate_token,
    get_auth_code,
    hash_pkce_verifier,
    revoke_refresh_token,
    store_access_token,
    store_auth_code,
    store_csrf_token,
    store_refresh_token,
    validate_csrf_token,
    validate_refresh_token,
)

limiter = Limiter(key_func=get_remote_address)
router = APIRouter(tags=["oauth"])


@router.get("/.well-known/oauth-authorization-server", response_model=OAuthMetadata)
async def oauth_metadata():
    """OAuth 2.0 Authorization Server Metadata (RFC 8414)."""
    return OAuthMetadata(
        issuer=settings.base_url,
        authorization_endpoint=f"{settings.base_url}/authorize",
        token_endpoint=f"{settings.base_url}/token",
        registration_endpoint=f"{settings.base_url}/register",
        response_types_supported=["code"],
        grant_types_supported=["authorization_code", "refresh_token"],
        code_challenge_methods_supported=["S256"],
    )


@router.post("/register", response_model=ClientRegistrationResponse)
@limiter.limit("10/hour")
async def register_client(request: Request, data: ClientRegistration):
    """Dynamic Client Registration (RFC 7591)."""
    # Clean up stale clients (unused for 90+ days)
    await delete_stale_clients()

    client_id = str(uuid.uuid4())

    db = await get_db()
    await db.execute(
        "INSERT INTO oauth_clients (client_id, client_name, redirect_uris) VALUES (?, ?, ?)",
        (client_id, data.client_name, json.dumps(data.redirect_uris)),
    )
    await db.commit()

    return ClientRegistrationResponse(
        client_id=client_id,
        client_name=data.client_name,
        redirect_uris=data.redirect_uris,
    )


@router.get("/authorize", response_class=HTMLResponse)
@limiter.limit("20/minute")
async def authorize_get(
    request: Request,
    client_id: str = Query(...),
    redirect_uri: str = Query(...),
    response_type: str = Query(...),
    code_challenge: str = Query(...),
    code_challenge_method: str = Query("S256"),
    state: str = Query(None),
):
    """Authorization endpoint - show login form."""
    # Validate client
    db = await get_db()
    cursor = await db.execute(
        "SELECT redirect_uris FROM oauth_clients WHERE client_id = ?",
        (client_id,),
    )
    row = await cursor.fetchone()

    if not row:
        raise HTTPException(status_code=400, detail="Invalid client_id")

    redirect_uris = json.loads(row["redirect_uris"])
    if redirect_uri not in redirect_uris:
        raise HTTPException(status_code=400, detail="Invalid redirect_uri")

    if response_type != "code":
        raise HTTPException(status_code=400, detail="Unsupported response_type")

    if code_challenge_method != "S256":
        raise HTTPException(status_code=400, detail="Unsupported code_challenge_method")

    # Generate CSRF token
    csrf_token = generate_token()
    await store_csrf_token(csrf_token)

    # Render login form - escape all user-controlled values to prevent XSS
    page = f"""
    <!DOCTYPE html>
    <html>
    <head>
        <title>Authorize - MyKB</title>
        <meta name="viewport" content="width=device-width, initial-scale=1">
        <style>
            body {{ font-family: system-ui, sans-serif; max-width: 400px; margin: 50px auto; padding: 20px; }}
            h1 {{ font-size: 1.5em; }}
            form {{ display: flex; flex-direction: column; gap: 15px; }}
            input {{ padding: 10px; font-size: 16px; border: 1px solid #ccc; border-radius: 4px; }}
            button {{ padding: 12px; font-size: 16px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; }}
            button:hover {{ background: #0056b3; }}
            .info {{ color: #666; font-size: 0.9em; }}
        </style>
    </head>
    <body>
        <h1>Authorize Access</h1>
        <p class="info">An application is requesting access to your MyKB data.</p>
        <form method="POST" action="/authorize">
            <input type="hidden" name="client_id" value="{html.escape(client_id)}">
            <input type="hidden" name="redirect_uri" value="{html.escape(redirect_uri)}">
            <input type="hidden" name="code_challenge" value="{html.escape(code_challenge)}">
            <input type="hidden" name="code_challenge_method" value="{html.escape(code_challenge_method)}">
            <input type="hidden" name="state" value="{html.escape(state or '')}">
            <input type="hidden" name="csrf_token" value="{html.escape(csrf_token)}">
            <input type="password" name="password" placeholder="Enter password" required autofocus>
            <button type="submit">Authorize</button>
        </form>
    </body>
    </html>
    """
    return HTMLResponse(content=page)


@router.post("/authorize")
@limiter.limit("5/minute")
async def authorize_post(
    request: Request,
    client_id: str = Form(...),
    redirect_uri: str = Form(...),
    code_challenge: str = Form(...),
    code_challenge_method: str = Form("S256"),
    state: str = Form(None),
    csrf_token: str = Form(...),
    password: str = Form(...),
):
    """Handle authorization form submission."""
    # Verify CSRF token
    if not await validate_csrf_token(csrf_token):
        raise HTTPException(status_code=400, detail="Invalid or expired CSRF token")

    # Verify password
    if not hmac.compare_digest(password, settings.auth_password):
        raise HTTPException(status_code=401, detail="Invalid password")

    # Generate authorization code
    code = generate_token()
    await store_auth_code(code, client_id, redirect_uri, code_challenge, code_challenge_method)

    # Redirect back to client
    params = {"code": code}
    if state:
        params["state"] = state

    redirect_url = f"{redirect_uri}?{urlencode(params)}"
    return RedirectResponse(url=redirect_url, status_code=302)


@router.post("/token", response_model=TokenResponse)
@limiter.limit("30/minute")
async def token(
    request: Request,
    grant_type: str = Form(...),
    code: str = Form(None),
    redirect_uri: str = Form(None),
    code_verifier: str = Form(None),
    client_id: str = Form(None),
    refresh_token: str = Form(None),
):
    """Token endpoint - exchange code or refresh token for access token."""
    if grant_type == "authorization_code":
        if not code or not code_verifier or not client_id or not redirect_uri:
            raise HTTPException(status_code=400, detail="Missing required parameters")

        # Get and validate authorization code
        code_data = await get_auth_code(code)
        if not code_data:
            raise HTTPException(status_code=400, detail="Invalid or expired code")

        if code_data["client_id"] != client_id:
            raise HTTPException(status_code=400, detail="Client ID mismatch")

        if code_data["redirect_uri"] != redirect_uri:
            raise HTTPException(status_code=400, detail="Redirect URI mismatch")

        # Verify PKCE
        expected_challenge = hash_pkce_verifier(code_verifier)
        if expected_challenge != code_data["code_challenge"]:
            raise HTTPException(status_code=400, detail="Invalid code_verifier")

        # Generate and store tokens
        access_token = generate_token()
        new_refresh_token = generate_token()
        await store_access_token(access_token, client_id)
        await store_refresh_token(new_refresh_token, client_id)

        # Track client usage for auto-expiry
        await touch_client(client_id)

        return TokenResponse(
            access_token=access_token,
            token_type="Bearer",
            expires_in=settings.token_expiry_seconds,
            refresh_token=new_refresh_token,
        )

    elif grant_type == "refresh_token":
        if not refresh_token:
            raise HTTPException(status_code=400, detail="Missing refresh_token")

        # Validate refresh token
        token_data = await validate_refresh_token(refresh_token)
        if not token_data:
            raise HTTPException(status_code=400, detail="Invalid or expired refresh_token")

        token_client_id = token_data["client_id"]

        # Revoke old refresh token (rotation)
        await revoke_refresh_token(refresh_token)

        # Generate new tokens
        access_token = generate_token()
        new_refresh_token = generate_token()
        await store_access_token(access_token, token_client_id)
        await store_refresh_token(new_refresh_token, token_client_id)

        # Track client usage for auto-expiry
        await touch_client(token_client_id)

        return TokenResponse(
            access_token=access_token,
            token_type="Bearer",
            expires_in=settings.token_expiry_seconds,
            refresh_token=new_refresh_token,
        )

    else:
        raise HTTPException(status_code=400, detail="Unsupported grant_type")
