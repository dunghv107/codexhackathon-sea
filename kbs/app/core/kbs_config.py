from functools import lru_cache
from pathlib import Path

from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.core.config import DEFAULT_ENV_FILE

SIM_BATCH = "olist_vn_sim"
RULES_PATH = Path(__file__).resolve().parents[1] / "rules" / "rules.json"


class KBSSettings(BaseSettings):
    model_config = SettingsConfigDict(env_file=DEFAULT_ENV_FILE, extra="ignore", hide_input_in_errors=True)

    database_url: SecretStr | None = Field(default=None, validation_alias="DATABASE_URL")


@lru_cache
def get_kbs_settings() -> KBSSettings:
    return KBSSettings()
