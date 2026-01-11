FROM python:3.13-slim

WORKDIR /app

# Install uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Copy project files
COPY pyproject.toml uv.lock ./
COPY app ./app

# Install dependencies
RUN uv sync --frozen --no-dev

# Create non-root user (high UID to avoid conflicts) and data directory
RUN useradd --no-log-init --create-home --uid 10000 appuser \
    && mkdir -p /data \
    && chown appuser:appuser /data

USER appuser

EXPOSE 8000

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
