from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, call, patch

import pytest

from app.core.kbs_config import SIM_BATCH, KBSSettings
from app.services import backfill

pytestmark = pytest.mark.component


async def test_backfill_requires_database_before_opening_resources() -> None:
    with (
        patch.object(backfill, "get_kbs_settings", return_value=KBSSettings()),
        pytest.raises(ValueError, match="DATABASE_URL"),
    ):
        await backfill.main()


@pytest.mark.parametrize("scoring_fails", [False, True])
async def test_backfill_pages_by_last_order_groups_shops_and_closes(scoring_fails: bool) -> None:
    connection = AsyncMock()
    cursor = connection.execute.return_value
    cursor.fetchall.side_effect = [
        [{"order_id": "a", "shop_id": "one"}, {"order_id": "b", "shop_id": "two"}, {"order_id": "c", "shop_id": "one"}],
        [],
    ]
    database = MagicMock()
    database.open = AsyncMock()
    database.close = AsyncMock()
    database.pool.connection.return_value.__aenter__.return_value = connection
    service = MagicMock()
    service.score = AsyncMock(return_value=[SimpleNamespace(risk_level="low")])
    if scoring_fails:
        service.score.side_effect = ValueError("fixture scoring failure")
    settings = KBSSettings.model_validate({"DATABASE_URL": "postgresql://fixture.invalid/test"})
    with (
        patch.object(backfill, "get_kbs_settings", return_value=settings),
        patch.object(backfill, "Database", return_value=database),
        patch.object(backfill, "ScoringService", return_value=service),
    ):
        if scoring_fails:
            with pytest.raises(ValueError, match="fixture scoring failure"):
                await backfill.main()
        else:
            await backfill.main()
            assert service.score.await_args_list == [call(["a", "c"], "one"), call(["b"], "two")]
            assert connection.execute.await_args_list[1].args[1] == (SIM_BATCH, "c")
    assert connection.execute.await_args_list[0].args[1] == (SIM_BATCH, "")
    database.open.assert_awaited_once()
    database.close.assert_awaited_once()
