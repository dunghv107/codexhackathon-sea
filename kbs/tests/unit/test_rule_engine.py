from copy import deepcopy
from decimal import Decimal

import pytest
from pydantic import ValidationError

from app.core.kbs_config import RULES_PATH
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.schemas.risk import Features

pytestmark = pytest.mark.unit


@pytest.fixture
def engine() -> RuleEngine:
    return RuleEngine(RuleConfig.load(RULES_PATH))


@pytest.mark.parametrize(("failed", "resolved", "points"), [(0, 2, 0), (1, 2, 15), (3, 4, 30), (8, 8, 45)])
def test_bayesian_tiers_fire_once(engine: RuleEngine, failed: int, resolved: int, points: int) -> None:
    result = engine.evaluate({"R1": {"failed": Decimal(failed), "resolved": Decimal(resolved)}})
    assert result.score == points
    assert len(result.reasons) == bool(points)


def test_unknown_is_preserved_without_mutating_facts(engine: RuleEngine) -> None:
    facts: Features = {"R1": {"resolved": Decimal(0), "failed": Decimal(0)}, "R8": {"age_days": None}}
    before = deepcopy(facts)
    result = engine.evaluate(facts)
    assert result.score == 0
    assert result.unknown_reasons["R1"] == "No resolved history"
    assert result.unknown_reasons["R8"] == "Unknown age_days"
    assert facts == before


@pytest.mark.parametrize(("amount", "age", "expected"), [(300, 1, 0), (301, 1, 25), (301, 7, 15)])
def test_strict_value_ratio_and_account_age(engine: RuleEngine, amount: int, age: int, expected: int) -> None:
    result = engine.evaluate(
        {
            "R7": {"cod": Decimal(1), "history_orders": Decimal(5), "median": Decimal(100), "amount": Decimal(amount)},
            "R8": {"age_days": Decimal(age)},
        }
    )
    assert result.score == expected


def test_dependency_order_and_cap(engine: RuleEngine) -> None:
    result = engine.evaluate(
        {
            "R1": {"failed": Decimal(8), "resolved": Decimal(8)},
            "R3": {"count": Decimal(3)},
            "R5": {"checkouts": Decimal(3)},
            "R8": {"age_days": Decimal(1)},
        }
    )
    assert result.score == 100
    assert result.risk_level == "high"
    assert [r.rule_id for r in result.reasons] == ["R1", "R3", "R5", "R8"]
    assert sum(r.points for r in result.reasons) == 120


def test_velocity_without_adverse_history_does_not_fire_r5(engine: RuleEngine) -> None:
    result = engine.evaluate({"R5": {"checkouts": Decimal(10)}, "R8": {"age_days": Decimal(1)}})
    assert result.score == 0


def test_configuration_changes_drive_points_and_thresholds(engine: RuleEngine) -> None:
    config = engine.config.model_dump()
    config["thresholds"]["medium_min"] = 10
    next(rule for rule in config["rules"] if rule["rule_id"] == "R10")["max_points"] = 11
    result = RuleEngine(RuleConfig.model_validate(config)).evaluate({"R10": {"count": Decimal(2)}})
    assert result.score == 11
    assert result.risk_level == "medium"


def test_config_rejects_broken_dependency_and_unsupported_address_rule(engine: RuleEngine) -> None:
    config = engine.config.model_dump()
    config["dependency_order"].reverse()
    with pytest.raises(ValidationError, match="Dependency"):
        RuleConfig.model_validate(config)
    config = engine.config.model_dump()
    next(rule for rule in config["rules"] if rule["rule_id"] == "R9")["enabled"] = True
    with pytest.raises(ValidationError, match="R9"):
        RuleConfig.model_validate(config)
