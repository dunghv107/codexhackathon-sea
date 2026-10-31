import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Any
from uuid import uuid4

from psycopg import AsyncConnection
from psycopg.types.json import Jsonb

from app.core.kbs_config import RULES_PATH, SIM_BATCH
from app.rules.config import RuleConfig
from app.schemas.risk import Assessment, OrderFacts, ScoreResult
from app.services.errors import ConflictError, NotFoundError


class RiskRepository:
    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.connection = connection

    async def initialize_rules(self) -> None:
        """Install the unchanged draft only for a batch with no configured versions."""
        raw = RULES_PATH.read_bytes()
        config = RuleConfig.model_validate_json(raw)
        await self.connection.execute(
            "INSERT INTO sfd.rule_versions "
            "(batch_id,rule_version,config,config_sha256,activated_at,approved_by) "
            "SELECT %s,%s,%s,%s,now(),'bootstrap:demo-draft' "
            "WHERE EXISTS (SELECT 1 FROM sfd.data_batches WHERE batch_id = %s) "
            "AND NOT EXISTS (SELECT 1 FROM sfd.rule_versions WHERE batch_id = %s) "
            "ON CONFLICT (rule_version) DO NOTHING",
            (SIM_BATCH, config.version, Jsonb(json.loads(raw)), hashlib.sha256(raw).hexdigest(), SIM_BATCH, SIM_BATCH),
        )

    async def configuration(self) -> RuleConfig:
        cursor = await self.connection.execute(
            "SELECT rule_version, config FROM sfd.rule_versions WHERE batch_id = %s AND activated_at <= now() "
            "ORDER BY activated_at DESC, rule_version DESC LIMIT 1",
            (SIM_BATCH,),
        )
        row = await cursor.fetchone()
        if row is None:
            raise ConflictError("No active rule version in simulation batch")
        config = RuleConfig.model_validate(row["config"])
        if config.version != row["rule_version"]:
            raise ConflictError("Stored rule version differs from configuration version")
        return config

    async def lock_orders(self, order_ids: list[str], shop_id: str) -> None:
        cursor = await self.connection.execute(
            "SELECT order_id FROM sfd.orders WHERE batch_id = %s AND shop_id = %s "
            "AND order_id = ANY(%s) ORDER BY order_id FOR UPDATE",
            (SIM_BATCH, shop_id, order_ids),
        )
        if len(await cursor.fetchall()) != len(set(order_ids)):
            raise NotFoundError("Order not found")

    async def facts(self, order_ids: list[str], shop_id: str, config: RuleConfig) -> list[OrderFacts]:
        parameters: dict[str, object] = {"batch": SIM_BATCH, "shop_id": shop_id, "order_ids": order_ids}
        for rule in config.rules:
            if rule.params.window_days is not None:
                parameters[f"{rule.rule_id.lower()}_days"] = rule.params.window_days
            if rule.params.window_hours is not None:
                parameters[f"{rule.rule_id.lower()}_hours"] = rule.params.window_hours
        query = (Path(__file__).resolve().parents[1] / "rules/sql/facts.sql").read_bytes()
        cursor = await self.connection.execute(query, parameters)
        return [OrderFacts.model_validate(row) for row in await cursor.fetchall()]

    async def previous(self, order_ids: list[str], shop_id: str, version: str) -> dict[str, Assessment]:
        cursor = await self.connection.execute(
            "SELECT DISTINCT ON (a.order_id) a.* FROM sfd.risk_assessments a "
            "JOIN sfd.orders o ON o.order_id = a.order_id AND o.batch_id = a.batch_id "
            "WHERE a.batch_id = %s AND o.shop_id = %s AND a.order_id = ANY(%s) AND a.rule_version = %s "
            "ORDER BY a.order_id, a.assessed_at DESC, a.assessment_id DESC",
            (SIM_BATCH, shop_id, order_ids, version),
        )
        return {row["order_id"]: Assessment.model_validate(row) for row in await cursor.fetchall()}

    async def save(self, facts: OrderFacts, config: RuleConfig, result: ScoreResult) -> Assessment:
        snapshot: dict[str, object] = result.model_dump(mode="json")["feature_snapshot"]
        snapshot["unknown_reasons"] = result.unknown_reasons
        assessment = Assessment(
            assessment_id=f"live:{uuid4()}",
            order_id=facts.order_id,
            rule_version=config.version,
            input_cutoff_at=facts.input_cutoff_at,
            assessed_at=datetime.now(UTC),
            score=result.score,
            risk_level=result.risk_level,
            reasons=result.reasons,
            feature_snapshot=snapshot,
        )
        await self.connection.execute(
            "INSERT INTO sfd.risk_assessments "
            "(batch_id, assessment_id, order_id, assessed_at, rule_version, score, risk_level, "
            "feature_snapshot, reasons, input_cutoff_at) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
            (
                SIM_BATCH,
                assessment.assessment_id,
                facts.order_id,
                assessment.assessed_at,
                config.version,
                result.score,
                result.risk_level,
                Jsonb(snapshot),
                Jsonb([reason.model_dump() for reason in result.reasons]),
                facts.input_cutoff_at,
            ),
        )
        await self.apply_review_policy(assessment, facts.shop_id)
        return assessment

    async def apply_review_policy(self, assessment: Assessment, shop_id: str) -> None:
        release = assessment.risk_level == "low"
        cursor = await self.connection.execute(
            "UPDATE sfd.orders o SET review_status = %(status)s "
            "WHERE o.order_id = %(order)s AND o.shop_id = %(shop)s AND o.batch_id = %(batch)s "
            "AND o.fulfillment_status = 'unfulfilled' AND o.review_status <> %(status)s "
            "AND %(assessment)s = (SELECT a.assessment_id FROM sfd.risk_assessments a "
            "WHERE a.order_id = o.order_id AND a.batch_id = o.batch_id "
            "ORDER BY a.assessed_at DESC, a.assessment_id DESC LIMIT 1) "
            # A policy release must never override the seller's latest Hold/Keep Hold.
            "AND (NOT %(release)s OR o.review_status = 'pending' OR "
            "(SELECT d.actor_type = 'policy' AND d.action IN ('hold', 'keep_hold') "
            "FROM sfd.review_decisions d WHERE d.order_id = o.order_id AND d.batch_id = o.batch_id "
            "ORDER BY d.created_at DESC, d.decision_id DESC LIMIT 1)) RETURNING o.order_id",
            {
                "status": "released" if release else "hold",
                "order": assessment.order_id,
                "shop": shop_id,
                "batch": SIM_BATCH,
                "assessment": assessment.assessment_id,
                "release": release,
            },
        )
        if await cursor.fetchone():
            await self.connection.execute(
                "INSERT INTO sfd.review_decisions "
                "(batch_id,decision_id,order_id,assessment_id,actor_type,actor_ref,action,reason,created_at) "
                "VALUES (%s,%s,%s,%s,'policy',%s,%s,%s,%s)",
                (
                    SIM_BATCH,
                    f"live:{uuid4()}",
                    assessment.order_id,
                    assessment.assessment_id,
                    assessment.rule_version,
                    "release" if release else "hold",
                    f"{assessment.risk_level.capitalize()} risk score",
                    datetime.now(UTC),
                ),
            )
