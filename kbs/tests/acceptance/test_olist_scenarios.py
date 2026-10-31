import hashlib
import json
import os
from pathlib import Path
from typing import Any

import pytest
from psycopg import AsyncConnection
from psycopg.rows import dict_row

from app.core.kbs_config import RULES_PATH, SIM_BATCH
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine

pytestmark = pytest.mark.acceptance
DATA = Path(__file__).resolve().parents[2] / "docs/data-structure/olist_cod/olist_cod"


@pytest.mark.parametrize("scenario_id", ["S01", "S02", "S03", "S04", "S05", "S06", "X01", "X04"])
async def test_dataset_contract(scenario_id: str) -> None:
    url = os.getenv("KBS_ACCEPTANCE_DATABASE_URL")
    if not url:
        pytest.skip("Set KBS_ACCEPTANCE_DATABASE_URL to an already-loaded olist_cod database")
    scenarios = json.loads((DATA / "scenarios.json").read_text(encoding="utf-8"))
    expected = json.loads((DATA / "evaluation/scenario_expectations.json").read_text(encoding="utf-8"))
    scenario = next(item for item in scenarios if item["scenario_id"] == scenario_id)
    config = RuleConfig.load(RULES_PATH)
    assert config.version == expected["rule_version"]
    assert hashlib.sha256(RULES_PATH.read_bytes()).hexdigest() == expected["rules_sha256"]
    async with await AsyncConnection[dict[str, Any]].connect(url, row_factory=dict_row) as connection:
        await connection.execute("SET TRANSACTION READ ONLY")
        cursor = await connection.execute(
            "SELECT shop_id FROM sfd.orders WHERE order_id = %s AND batch_id = %s",
            (scenario["order_id"], SIM_BATCH),
        )
        row = await cursor.fetchone()
        assert row is not None, "Dataset scenario is missing"
        facts = (await RiskRepository(connection).facts([scenario["order_id"]], row["shop_id"], config))[0]
        result = RuleEngine(config).evaluate(facts.features)
    contract = expected["scenarios"][scenario_id]
    assert result.score == contract["score"]
    assert result.risk_level == contract["risk_level"]
    assert {r.rule_id for r in result.reasons} == set(contract["fired_rules"])
