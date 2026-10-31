from typing import Annotated, Literal

import httpx
from fastapi import APIRouter, HTTPException, Query, Request

from app.api.kbs_dependencies import Connection, Networks, Orders, Scoring, Shop
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.schemas.network import OrderNetwork
from app.schemas.orders import Explanation, OrderDetail, OrderSummary
from app.schemas.risk import Assessment, RiskLevel
from app.services.voice_service import VoiceInput, VoiceResult

router = APIRouter(tags=["risk"])


@router.post("/orders/{order_id}/voice-preview")
async def voice_preview(order_id: str, body: VoiceInput, shop: Shop, service: Orders, request: Request) -> VoiceResult:
    await service.detail(order_id, shop)
    if request.app.state.voice is None:
        raise HTTPException(503, "Voice recognition is not configured")
    try:
        return await request.app.state.voice.preview(body)
    except (httpx.HTTPError, TimeoutError, ValueError, TypeError, KeyError, AttributeError):
        raise HTTPException(503, "Voice recognition unavailable; retry or use keypad") from None


@router.get("/orders")
async def list_orders(
    shop: Shop,
    service: Orders,
    risk_level: RiskLevel | None = None,
    review_status: Literal["pending", "hold", "released"] | None = None,
    limit: Annotated[int, Query(ge=1, le=100)] = 50,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> list[OrderSummary]:
    return await service.list_orders(shop, risk_level, review_status, limit, offset)


@router.get("/orders/{order_id}")
async def get_order(order_id: str, shop: Shop, service: Orders) -> OrderDetail:
    return await service.detail(order_id, shop)


@router.post("/orders/{order_id}/score")
async def score_order(order_id: str, shop: Shop, service: Scoring, force: bool = False) -> Assessment:
    return (await service.score([order_id], shop, force=force))[0]


@router.post("/orders/{order_id}/explain")
async def explain_order(order_id: str, shop: Shop, service: Orders) -> Explanation:
    return await service.explain(order_id, shop)


@router.get("/orders/{order_id}/network")
async def order_network(order_id: str, shop: Shop, service: Networks) -> OrderNetwork:
    return await service.network(order_id, shop)


@router.get("/rules")
async def get_rules(shop: Shop, conn: Connection) -> RuleConfig:
    del shop
    return await RiskRepository(conn).configuration()


@router.get("/rules/versions")
async def get_rule_versions(shop: Shop, conn: Connection) -> list[dict[str, object]]:
    del shop
    return await OrderRepository(conn).rule_versions()
