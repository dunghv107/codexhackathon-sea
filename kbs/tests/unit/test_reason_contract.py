import pytest

from app.schemas.risk import Reason


@pytest.mark.parametrize("payload", [{"contribution": 40}, {"points": 40}, {"contribution": 40, "message": "Evidence"}])
def test_stored_reason_variants_use_canonical_contribution(payload: dict[str, object]) -> None:
    reason = Reason.model_validate({"rule_id": "R3", **payload})
    assert reason.contribution == 40
    assert reason.points == 40
    assert reason.model_dump()["contribution"] == 40
    assert "points" not in reason.model_dump()


def test_zero_contribution_is_preserved() -> None:
    assert Reason.model_validate({"rule_id": "R9", "contribution": 0}).contribution == 0
