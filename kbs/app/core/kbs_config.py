from functools import lru_cache
from pathlib import Path
from urllib.parse import urlsplit

from pydantic import Field, SecretStr, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.core.config import DEFAULT_ENV_FILE

SIM_BATCH = "olist_vn_sim"
RULES_PATH = Path(__file__).resolve().parents[1] / "rules" / "rules.json"


class KBSSettings(BaseSettings):
    model_config = SettingsConfigDict(env_file=DEFAULT_ENV_FILE, extra="ignore", hide_input_in_errors=True)

    database_url: SecretStr | None = Field(default=None, validation_alias="DATABASE_URL")
    explain_enabled: bool = Field(default=False, validation_alias="KBS_EXPLAIN_ENABLED")
    explain_model: str = Field(default="", validation_alias="KBS_EXPLAIN_MODEL")
    asr_model: str = Field(default="", validation_alias="KBS_ASR_MODEL")
    llm_base_url: str = Field(default="https://api.openai.com/v1", validation_alias="KBS_LLM_BASE_URL")
    openai_api_key: SecretStr | None = Field(default=None, validation_alias="OPENAI_API_KEY")

    @model_validator(mode="after")
    def validate_explain(self) -> "KBSSettings":
        if self.asr_model.strip() and not self.explain_enabled:
            raise ValueError("ASR requires enabled explanations for intent classification")
        url = urlsplit(self.llm_base_url)
        if (
            url.scheme not in {"http", "https"}
            or not url.hostname
            or url.username
            or url.password
            or url.query
            or url.fragment
        ):
            raise ValueError("KBS_LLM_BASE_URL must be an HTTP(S) base URL without credentials, query or fragment")
        if self.explain_enabled and (
            not self.explain_model.strip()
            or not self.openai_api_key
            or not self.openai_api_key.get_secret_value().strip()
        ):
            raise ValueError("Enabled explanations require KBS_EXPLAIN_MODEL and OPENAI_API_KEY")
        return self


@lru_cache
def get_kbs_settings() -> KBSSettings:
    return KBSSettings()
