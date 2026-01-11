FROM python:3.13-slim

# Install uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Create non-root user and directories
RUN useradd --no-log-init --create-home --uid 10000 appuser \
    && mkdir -p /data /app \
    && chown appuser:appuser /data /app

USER appuser
WORKDIR /app

# Copy project files
COPY --chown=appuser:appuser pyproject.toml uv.lock ./
COPY --chown=appuser:appuser app ./app

# Install dependencies
RUN uv sync --frozen --no-dev

EXPOSE 8000

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
