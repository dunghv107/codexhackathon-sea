from datetime import datetime
from typing import Literal

from pydantic import BaseModel, Field

from app.schemas.risk import Assessment, RiskLevel


class OrderSummary(BaseModel):
    order_id: str
    checkout_id: str
    shop_id: str
    placed_at: datetime
    fulfillment_status: str
    review_status: Literal["pending", "hold", "released"]
    score: int | None = None
    risk_level: RiskLevel | None = None


class OrderEvent(BaseModel):
    event_type: str
    actor_type: str
    reason: str | None
    occurred_at: datetime
    available_at: datetime | None


class ReviewDecision(BaseModel):
    action: Literal["hold", "release", "keep_hold"]
    actor_type: str
    actor_ref: str
    reason: str
    created_at: datetime


class OrderDetail(OrderSummary):
    assessment: Assessment | None
    events: list[OrderEvent]
    decisions: list[ReviewDecision]


class Explanation(BaseModel):
    assessment_id: str
    source: Literal["template", "llm"] = "template"
    sentences: list[str] = Field(min_length=1, max_length=3)
    fallback_reason: (
        Literal[
            "disabled",
            "no_fired_rules",
            "provider_timeout",
            "provider_unavailable",
            "provider_refusal",
            "invalid_output",
        ]
        | None
    ) = None
