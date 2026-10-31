from typing import Any
from uuid import uuid4

from psycopg import AsyncConnection

from app.core.kbs_config import SIM_BATCH
from app.schemas.orders import DecisionRequest, OrderDetail, OrderSummary, ReviewDecision
from app.schemas.risk import Assessment, RiskLevel
from app.services.errors import ConflictError, NotFoundError


class OrderRepository:
    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.connection = connection

    async def list_orders(
        self, shop_id: str, risk_level: RiskLevel | None, review_status: str | None, limit: int, offset: int
    ) -> list[OrderSummary]:
        cursor = await self.connection.execute(
            "SELECT o.*, c.placed_at, a.score, a.risk_level FROM sfd.orders o "
            "JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id "
            "LEFT JOIN LATERAL (SELECT score, risk_level FROM sfd.risk_assessments a "
            "WHERE a.order_id = o.order_id AND a.batch_id = o.batch_id "
            "ORDER BY assessed_at DESC, assessment_id DESC LIMIT 1) a ON true "
            "WHERE o.batch_id = %(batch)s AND o.shop_id = %(shop)s "
            "AND (%(risk)s::text IS NULL OR a.risk_level = %(risk)s) "
            "AND (%(review)s::text IS NULL OR o.review_status = %(review)s) "
            "ORDER BY c.placed_at DESC, o.order_id LIMIT %(limit)s OFFSET %(offset)s",
            {
                "batch": SIM_BATCH,
                "shop": shop_id,
                "risk": risk_level,
                "review": review_status,
                "limit": limit,
                "offset": offset,
            },
        )
        return [OrderSummary.model_validate(row) for row in await cursor.fetchall()]

    async def detail(self, order_id: str, shop_id: str) -> OrderDetail:
        cursor = await self.connection.execute(
            "SELECT o.*, c.placed_at FROM sfd.orders o JOIN sfd.checkouts c "
            "ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id "
            "WHERE o.order_id = %s AND o.shop_id = %s AND o.batch_id = %s",
            (order_id, shop_id, SIM_BATCH),
        )
        row = await cursor.fetchone()
        if row is None:
            raise NotFoundError("Order not found")
        cursor = await self.connection.execute(
            "SELECT * FROM sfd.risk_assessments WHERE order_id = %s AND batch_id = %s "
            "ORDER BY assessed_at DESC, assessment_id DESC LIMIT 1",
            (order_id, SIM_BATCH),
        )
        assessment_row = await cursor.fetchone()
        assessment = Assessment.model_validate(assessment_row) if assessment_row else None
        cursor = await self.connection.execute(
            "SELECT event_type, actor_type, reason, occurred_at, available_at FROM sfd.order_events "
            "WHERE order_id = %s AND batch_id = %s ORDER BY occurred_at, event_id",
            (order_id, SIM_BATCH),
        )
        events = await cursor.fetchall()
        cursor = await self.connection.execute(
            "SELECT action, actor_type, actor_ref, reason, created_at FROM sfd.review_decisions "
            "WHERE order_id = %s AND batch_id = %s ORDER BY created_at, decision_id",
            (order_id, SIM_BATCH),
        )
        return OrderDetail.model_validate(
            {
                **row,
                "assessment": assessment,
                "events": events,
                "decisions": await cursor.fetchall(),
                "score": assessment.score if assessment else None,
                "risk_level": assessment.risk_level if assessment else None,
            }
        )

    async def decide(self, order_id: str, shop_id: str, request: DecisionRequest) -> ReviewDecision:
        cursor = await self.connection.execute(
            "SELECT review_status, fulfillment_status FROM sfd.orders "
            "WHERE order_id = %s AND shop_id = %s AND batch_id = %s FOR UPDATE",
            (order_id, shop_id, SIM_BATCH),
        )
        order = await cursor.fetchone()
        if order is None:
            raise NotFoundError("Order not found")
        if order["fulfillment_status"] != "unfulfilled":
            raise ConflictError("Review decisions require an unfulfilled order")
        if request.action == "keep_hold" and order["review_status"] != "hold":
            raise ConflictError("keep_hold requires an order already on hold")
        cursor = await self.connection.execute(
            "SELECT assessment_id FROM sfd.risk_assessments WHERE order_id = %s AND batch_id = %s "
            "ORDER BY assessed_at DESC, assessment_id DESC LIMIT 1",
            (order_id, SIM_BATCH),
        )
        assessment = await cursor.fetchone()
        if assessment is None:
            raise ConflictError("Score the order before making a review decision")
        await self.connection.execute(
            "UPDATE sfd.orders SET review_status = %s WHERE order_id = %s AND shop_id = %s AND batch_id = %s",
            ("released" if request.action == "release" else "hold", order_id, shop_id, SIM_BATCH),
        )
        cursor = await self.connection.execute(
            "INSERT INTO sfd.review_decisions "
            "(batch_id,decision_id,order_id,assessment_id,actor_type,actor_ref,action,reason,created_at) "
            "VALUES (%s,%s,%s,%s,'seller',%s,%s,%s,clock_timestamp()) "
            "RETURNING action, actor_type, actor_ref, reason, created_at",
            (
                SIM_BATCH,
                f"live:{uuid4()}",
                order_id,
                assessment["assessment_id"],
                shop_id,
                request.action,
                request.reason,
            ),
        )
        return ReviewDecision.model_validate(await cursor.fetchone())

    async def rule_versions(self) -> list[dict[str, object]]:
        cursor = await self.connection.execute(
            "SELECT rule_version, config_sha256, activated_at, approved_by FROM sfd.rule_versions "
            "WHERE batch_id = %s ORDER BY activated_at DESC, rule_version DESC",
            (SIM_BATCH,),
        )
        return list(await cursor.fetchall())
