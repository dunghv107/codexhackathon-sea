import os
from unittest.mock import AsyncMock, patch

import pytest
from httpx import ASGITransport, AsyncClient
from psycopg import OperationalError

from app.api.app import create_app
from app.core.kbs_config import KBSSettings
from app.db.postgres import Database


@pytest.mark.integration
async def test_app_opens_database_readiness_and_closes_pool() -> None:
    url = os.getenv("KBS_TEST_DATABASE_URL")
    if not url:
        pytest.skip("KBS_TEST_DATABASE_URL requires an isolated database")
    settings = KBSSettings.model_validate({"DATABASE_URL": url})
    with patch("app.api.app.get_kbs_settings", return_value=settings):
        app = create_app()
    database = app.state.database
    assert database.pool.closed
    async with app.router.lifespan_context(app):
        assert not database.pool.closed
        async with AsyncClient(transport=ASGITransport(app), base_url="http://test") as client:
            assert (await client.get("/ready")).json() == {"status": "ok"}
            assert (await client.get("/health")).status_code == 200
    assert database.pool.closed


@pytest.mark.component
async def test_failed_database_startup_closes_pool() -> None:
    database = Database("postgresql://fixture.invalid/test")
    pool = AsyncMock()
    pool.wait.side_effect = OperationalError("fixture database unavailable")
    with patch.object(database, "pool", pool), pytest.raises(OperationalError):
        await database.open()
    pool.open.assert_awaited_once()
    pool.close.assert_awaited_once()
