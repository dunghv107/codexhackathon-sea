from datetime import datetime
from decimal import Decimal
from typing import Literal

from pydantic import AliasChoices, BaseModel, Field

from app.rules.config import RuleId

RiskLevel = Literal["low", "medium", "high"]
Features = dict[str, dict[str, Decimal | None]]


class OrderFacts(BaseModel):
    order_id: str
    shop_id: str
    input_cutoff_at: datetime
    features: Features


class Reason(BaseModel):
    rule_id: RuleId
    contribution: int = Field(ge=0, validation_alias=AliasChoices("contribution", "points"))
    description: str = Field(default="", validation_alias=AliasChoices("description", "message"))

    @property
    def points(self) -> int:
        return self.contribution


class ScoreResult(BaseModel):
    score: int = Field(ge=0, le=100)
    risk_level: RiskLevel
    reasons: list[Reason]
    feature_snapshot: Features
    unknown_reasons: dict[str, str]


class Assessment(BaseModel):
    assessment_id: str
    order_id: str
    rule_version: str
    input_cutoff_at: datetime
    assessed_at: datetime
    score: int
    risk_level: RiskLevel
    reasons: list[Reason]
    feature_snapshot: dict[str, object]
    matches_previous: bool | None = None
