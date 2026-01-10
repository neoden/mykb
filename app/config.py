from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Server
    host: str = "0.0.0.0"
    port: int = 8000
    base_url: str  # Required, set via BASE_URL env var

    # Auth
    auth_password: str = "changeme"
    token_expiry_seconds: int = 3600  # 1 hour
    refresh_token_expiry_seconds: int = 2592000  # 30 days
    code_expiry_seconds: int = 300  # 5 minutes

    # Database
    database_path: str = "/data/mykb.db"

    # Redis
    redis_url: str = "redis://localhost:6379"

    model_config = {"env_file": ".env", "env_file_encoding": "utf-8"}


settings = Settings()
