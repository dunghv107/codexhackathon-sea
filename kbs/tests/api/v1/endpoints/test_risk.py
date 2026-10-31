from unittest.mock import AsyncMock

import pytest
from httpx import ASGITransport, AsyncClient
from psycopg import OperationalError

from app.api.app import create_app

pytestmark = pytest.mark.component


async def test_disabled_database_does_not_break_health() -> None:
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app), base_url="http://test") as client:
        assert (await client.get("/health")).status_code == 200
        assert (await client.get("/ready")).status_code == 200
        response = await client.get("/api/v1/orders", headers={"X-Shop-ID": "shop"})
        assert response.status_code == 503
        assert response.json()["detail"] == "KBS database is not configured"


async def test_configured_database_failure_only_fails_readiness() -> None:
    app = create_app()
    app.state.database = AsyncMock()
    app.state.database.ready.side_effect = OperationalError("private connection failure")
    async with AsyncClient(transport=ASGITransport(app), base_url="http://test") as client:
        assert (await client.get("/health")).status_code == 200
        for path in ("/ready", "/api/v1/health/ready"):
            response = await client.get(path)
            assert response.status_code == 503
            assert "private" not in response.text
