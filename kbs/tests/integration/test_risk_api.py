from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

import pytest
from httpx import ASGITransport, AsyncClient
from psycopg import AsyncConnection
from tests.integration.conftest import Dataset

from app.api.app import create_app

pytestmark = pytest.mark.integration


class TransactionPool:
    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.conn = connection

    @asynccontextmanager
    async def connection(self) -> AsyncIterator[AsyncConnection[dict[str, Any]]]:
        async with self.conn.transaction():
            yield self.conn


class TestDatabase:
    __test__ = False

    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.pool = TransactionPool(connection)


async def test_http_scoring_tenant_boundary_and_decision_validation(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    app = create_app()
    app.state.database = TestDatabase(dataset.connection)
    async with AsyncClient(
        transport=ASGITransport(app), base_url="http://test", headers={"X-Shop-ID": "shop-a"}
    ) as client:
        response = await client.post(f"/api/v1/orders/{target}/score")
        assert response.status_code == 200, response.text
        assert response.json()["score"] == 0
        assessment_id = response.json()["assessment_id"]
        response = await client.post(f"/api/v1/orders/{target}/score", headers={"X-Shop-ID": "shop-b"})
        assert response.status_code == 404
        response = await client.get(f"/api/v1/orders/{target}", headers={"X-Shop-ID": "shop-b"})
        assert response.status_code == 404
        response = await client.post(
            f"/api/v1/orders/{target}/decision", json={"action": "keep_hold", "reason": "Review"}
        )
        assert response.status_code == 404
        detail = (await client.get(f"/api/v1/orders/{target}")).json()
        assert detail["review_status"] == "pending"
        assert detail["assessment"]["assessment_id"] == assessment_id
        assert len(detail["decisions"]) == 0
        listing = await client.get("/api/v1/orders?risk_level=low&review_status=pending")
        assert [item["order_id"] for item in listing.json()] == [target]
        assert (await client.get("/api/v1/rules")).json()["version"] == "draft-vn-olist-0.2"
        assert len((await client.get("/api/v1/rules/versions")).json()) == 1
        assert (await client.post(f"/api/v1/orders/{target}/explain")).json()["source"] == "template"
        response = await client.get("/api/v1/orders", headers={"X-Shop-ID": "raw-shop"})
        assert response.status_code == 404
        client.headers.pop("X-Shop-ID")
        assert (await client.get("/api/v1/orders")).status_code == 422
