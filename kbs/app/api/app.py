from collections.abc import AsyncIterator
from contextlib import AsyncExitStack, asynccontextmanager

import httpx
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from mcp.server.transport_security import TransportSecuritySettings
from scalar_fastapi import get_scalar_api_reference

from app.agents.explain import ExplainAgent
from app.api.dependencies import get_api_settings
from app.api.kbs_readiness import check_database_ready
from app.api.v1 import api_router
from app.core.config import get_mcp_settings
from app.core.kbs_config import get_kbs_settings
from app.core.logging import configure_logging
from app.core.request_context import attach_request_id
from app.db.postgres import Database
from app.mcp import create_server
from app.mcp.http import BearerAuthMiddleware
from app.repositories.risk_repository import RiskRepository
from app.services import get_health_status, get_readiness_status
from app.services.explanation_service import ExplanationService
from app.services.voice_service import VoiceService


def _parse_cors_values(raw_value: str, *, default_wildcard: bool = False) -> list[str]:
    if raw_value == "*":
        return ["*"]

    values = [value.strip() for value in raw_value.split(",") if value.strip()]
    if values:
        return values

    return ["*"] if default_wildcard else []


@asynccontextmanager
async def lifespan(_app: FastAPI) -> AsyncIterator[None]:
    configure_logging()
    async with AsyncExitStack() as stack:
        kbs_settings = get_kbs_settings()
        if kbs_settings.explain_enabled and kbs_settings.openai_api_key:
            client = await stack.enter_async_context(
                httpx.AsyncClient(
                    headers={"Authorization": f"Bearer {kbs_settings.openai_api_key.get_secret_value()}"},
                    timeout=12,
                    follow_redirects=False,
                )
            )
            _app.state.explainer = ExplanationService(
                ExplainAgent(client, kbs_settings.explain_model, kbs_settings.llm_base_url)
            )
            if kbs_settings.asr_model.strip():
                _app.state.voice = VoiceService(
                    client, kbs_settings.llm_base_url, kbs_settings.asr_model, kbs_settings.explain_model
                )
        if _app.state.database is not None:
            await _app.state.database.open()
            stack.push_async_callback(_app.state.database.close)
            async with _app.state.database.pool.connection() as connection:
                await RiskRepository(connection).initialize_rules()
        if _app.state.mcp_server is not None:
            await stack.enter_async_context(_app.state.mcp_server.session_manager.run())
        yield


def create_app() -> FastAPI:
    settings = get_api_settings()
    mcp_settings = get_mcp_settings()
    if settings.environment == "production" and settings.cors_allow_origins == "*":
        raise ValueError("CORS_ALLOW_ORIGINS must be explicit in production")

    app = FastAPI(
        title=settings.app_name,
        version=settings.app_version,
        description="FastAPI template scaffold for backend APIs.",
        docs_url="/docs" if settings.docs_enabled else None,
        redoc_url=None,
        openapi_url=settings.openapi_url if settings.docs_enabled else None,
        lifespan=lifespan,
        openapi_tags=[
            {"name": "health", "description": "Public health-check endpoints."},
            {"name": "system", "description": "Application metadata and system endpoints."},
        ],
    )
    app.state.request_id_header = settings.request_id_header
    app.state.mcp_server = None
    database_url = get_kbs_settings().database_url
    app.state.database = Database(database_url.get_secret_value()) if database_url else None
    app.state.explainer = ExplanationService()
    app.state.voice = None
    app.middleware("http")(attach_request_id)

    app.add_middleware(
        CORSMiddleware,
        allow_origins=_parse_cors_values(settings.cors_allow_origins, default_wildcard=True),
        allow_credentials=settings.cors_allow_credentials,
        allow_methods=_parse_cors_values(settings.cors_allow_methods, default_wildcard=True),
        allow_headers=_parse_cors_values(settings.cors_allow_headers, default_wildcard=True),
        expose_headers=_parse_cors_values(settings.cors_expose_headers),
        max_age=settings.cors_max_age,
    )

    app.include_router(api_router, prefix=settings.api_prefix)

    if mcp_settings.enabled:
        server = create_server(settings)
        mcp_app = server.streamable_http_app(
            streamable_http_path="/",
            json_response=True,
            stateless_http=True,
            transport_security=TransportSecuritySettings(
                enable_dns_rebinding_protection=True,
                allowed_hosts=[value.strip() for value in mcp_settings.allowed_hosts.split(",") if value.strip()],
                allowed_origins=[value.strip() for value in mcp_settings.allowed_origins.split(",") if value.strip()],
            ),
        )
        if mcp_settings.bearer_token is None:
            raise ValueError("MCP_BEARER_TOKEN is required when MCP_ENABLED is true")
        app.mount("/mcp", BearerAuthMiddleware(mcp_app, mcp_settings.bearer_token), name="mcp")
        app.state.mcp_server = server

    @app.get("/health", include_in_schema=False)
    async def health_check() -> object:
        return get_health_status()

    @app.get("/ready", include_in_schema=False)
    async def readiness_check() -> object:
        await check_database_ready(app)
        return get_readiness_status()

    if settings.docs_enabled:

        @app.get(settings.scalar_path, include_in_schema=False)
        async def scalar_html() -> object:
            return get_scalar_api_reference(
                openapi_url=settings.openapi_url,
                title=f"{app.title} - Scalar",
            )

    return app


__all__ = ["create_app"]
