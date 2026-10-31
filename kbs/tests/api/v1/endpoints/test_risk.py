from datetime import UTC, datetime
from unittest.mock import AsyncMock, MagicMock

import pytest
from httpx import ASGITransport, AsyncClient
from psycopg import IntegrityError, OperationalError
from psycopg_pool import PoolTimeout

from app.api.app import create_app
from app.api.kbs_dependencies import scoring_service
from app.schemas.risk import Assessment
from app.services.errors import ConflictError

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


@pytest.mark.parametrize(
    ("failure", "status", "message"),
    [
        (ConflictError("Order data is incomplete"), 409, "Order data is incomplete"),
        (IntegrityError("private constraint name"), 409, "Operation conflicts with stored data"),
        (OperationalError("private database URL"), 503, "KBS database unavailable"),
        (PoolTimeout("private pool URL"), 503, "KBS database unavailable"),
    ],
)
async def test_database_failures_are_mapped_without_leaking_details(
    failure: Exception, status: int, message: str
) -> None:
    app = create_app()
    database = MagicMock()
    database.pool.connection.return_value.__aenter__.side_effect = failure
    app.state.database = database
    async with AsyncClient(transport=ASGITransport(app), base_url="http://test") as client:
        response = await client.get("/api/v1/orders", headers={"X-Shop-ID": "shop"})
    assert response.status_code == status
    assert response.json() == {"detail": message}
    assert "private" not in response.text


@pytest.mark.parametrize(
    ("commit_failure", "status", "message"),
    [
        (None, 200, None),
        (OperationalError("private commit connection details"), 503, "KBS database unavailable"),
        (IntegrityError("private deferred constraint"), 409, "Operation conflicts with stored data"),
    ],
)
async def test_scoring_finishes_transaction_before_sending_success(
    commit_failure: Exception | None, status: int, message: str | None
) -> None:
    app = create_app()
    database = MagicMock()
    transaction = database.pool.connection.return_value
    conn = AsyncMock()
    conn.execute.return_value.fetchone.return_value = {"exists": 1}
    transaction.__aenter__.return_value = conn
    transaction.__aexit__.side_effect = commit_failure
    app.state.database = database

    assessment = Assessment(
        assessment_id="assessment-before-commit",
        order_id="order-1",
        rule_version="v1",
        input_cutoff_at=datetime(2026, 1, 1, tzinfo=UTC),
        assessed_at=datetime(2026, 1, 1, tzinfo=UTC),
        score=0,
        risk_level="low",
        reasons=[],
        feature_snapshot={},
    )
    service = MagicMock()
    service.score = AsyncMock(return_value=[assessment])
    app.dependency_overrides[scoring_service] = lambda: service

    async with AsyncClient(transport=ASGITransport(app), base_url="http://test") as client:
        response = await client.post("/api/v1/orders/order-1/score", headers={"X-Shop-ID": "shop"})

    service.score.assert_awaited_once_with(["order-1"], "shop", force=False)
    transaction.__aexit__.assert_awaited_once_with(None, None, None)
    assert response.status_code == status
    if commit_failure is None:
        assert response.json() == assessment.model_dump(mode="json")
    else:
        assert response.json() == {"detail": message}
        assert assessment.assessment_id not in response.text
        assert "private" not in response.text
