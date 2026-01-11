import logging
from pathlib import Path

import aiosqlite

from app.config import settings

logger = logging.getLogger(__name__)

SCHEMA = """
-- Main chunks table
CREATE TABLE IF NOT EXISTS chunks (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    metadata JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- FTS5 virtual table for search (includes metadata)
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    id,
    content,
    metadata,
    content='chunks',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, id, content, metadata) VALUES (NEW.rowid, NEW.id, NEW.content, NEW.metadata);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, id, content, metadata) VALUES('delete', OLD.rowid, OLD.id, OLD.content, OLD.metadata);
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, id, content, metadata) VALUES('delete', OLD.rowid, OLD.id, OLD.content, OLD.metadata);
    INSERT INTO chunks_fts(rowid, id, content, metadata) VALUES (NEW.rowid, NEW.id, NEW.content, NEW.metadata);
END;

-- OAuth clients table
CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id TEXT PRIMARY KEY,
    client_name TEXT,
    redirect_uris JSON NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Migration tracking
CREATE TABLE IF NOT EXISTS migrations (
    id TEXT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
"""

# Migrations: list of (id, sql) tuples. IDs must be unique and never change.
MIGRATIONS = [
    ("001_oauth_clients_last_used_at", "ALTER TABLE oauth_clients ADD COLUMN last_used_at TIMESTAMP"),
]


_db: aiosqlite.Connection | None = None


async def get_db() -> aiosqlite.Connection:
    """Get database connection (singleton)."""
    global _db
    if _db is None:
        db_path = Path(settings.database_path)
        db_path.parent.mkdir(parents=True, exist_ok=True)
        _db = await aiosqlite.connect(db_path)
        _db.row_factory = aiosqlite.Row
    return _db


async def init_db(db: aiosqlite.Connection):
    """Initialize the database schema and run pending migrations."""
    await db.executescript(SCHEMA)
    await db.commit()

    # Get applied migrations
    cursor = await db.execute("SELECT id FROM migrations")
    applied = {row[0] for row in await cursor.fetchall()}

    # Run pending migrations
    for migration_id, sql in MIGRATIONS:
        if migration_id in applied:
            continue
        logger.info(f"Applying migration: {migration_id}")
        await db.execute(sql)
        await db.execute("INSERT INTO migrations (id) VALUES (?)", (migration_id,))
        await db.commit()


async def touch_client(client_id: str) -> None:
    """Update last_used_at timestamp for a client."""
    db = await get_db()
    await db.execute(
        "UPDATE oauth_clients SET last_used_at = CURRENT_TIMESTAMP WHERE client_id = ?",
        (client_id,),
    )
    await db.commit()


async def delete_stale_clients(max_age_days: int = 90) -> int:
    """Delete clients not used in max_age_days. Returns count deleted."""
    db = await get_db()
    cursor = await db.execute(
        """
        DELETE FROM oauth_clients
        WHERE last_used_at IS NULL AND created_at < datetime('now', ?)
           OR last_used_at < datetime('now', ?)
        """,
        (f"-{max_age_days} days", f"-{max_age_days} days"),
    )
    await db.commit()
    return cursor.rowcount
