from datetime import timedelta

import pytest
from tests.integration.conftest import CUTOFF, Dataset

from app.repositories.network_repository import NetworkRepository
from app.services.errors import NotFoundError
from app.services.network_service import NetworkService

pytestmark = pytest.mark.integration


async def test_network_cutoff_batch_and_cross_shop_redaction(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    await dataset.checkout("own", at=CUTOFF - timedelta(days=1))
    await dataset.checkout("secret-other", shops=("shop-b", "shop-c"), at=CUTOFF - timedelta(days=1))
    await dataset.checkout("future", at=CUTOFF + timedelta(seconds=1))
    await dataset.checkout("equal-time")
    await dataset.checkout("late-identity", at=CUTOFF - timedelta(days=1))
    await dataset.connection.execute(
        "UPDATE sfd.checkout_identity_snapshots SET available_at = %s WHERE checkout_id = 'late-identity'",
        (CUTOFF,),
    )
    await dataset.checkout("raw", shops=("raw-shop",), batch="olist_raw", at=CUTOFF - timedelta(days=1))
    service = NetworkService(NetworkRepository(dataset.connection))
    result = await service.network(target, "shop-a")
    assert result.input_cutoff_at == CUTOFF
    assert not result.truncated
    assert len([node for node in result.nodes if node.kind == "checkout"]) == 2
    assert [node.count for node in result.nodes if node.kind == "aggregate"] == [1, 1, 1]
    assert "secret-other" not in result.model_dump_json()
    assert "shop-b" not in result.model_dump_json()
    assert "phone_key" not in result.model_dump_json()
    with pytest.raises(NotFoundError):
        await service.network(target, "shop-b")
    with pytest.raises(NotFoundError):
        await service.network("raw:raw-shop", "raw-shop")


async def test_network_unknown_keys_and_late_target_identity(dataset: Dataset) -> None:
    service = NetworkService(NetworkRepository(dataset.connection))
    await dataset.checkout("history", at=CUTOFF - timedelta(days=1), phone=None, device=None, cluster=None)
    target = (await dataset.checkout("target", phone=None, device=None, cluster=None))[0]
    result = await service.network(target, "shop-a")
    assert len(result.nodes) == 1
    assert result.edges == []
    late = (await dataset.checkout("late"))[0]
    await dataset.connection.execute(
        "UPDATE sfd.checkout_identity_snapshots SET available_at = %s WHERE checkout_id = 'late'",
        (CUTOFF + timedelta(seconds=1),),
    )
    assert len((await service.network(late, "shop-a")).nodes) == 1


async def test_network_caps_nodes_and_counts_checkout_once(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    for index in range(194):
        await dataset.checkout(f"history-{index}", shops=("shop-a", "shop-b"), at=CUTOFF - timedelta(days=1))
    await dataset.checkout("other", shops=("shop-b",), at=CUTOFF - timedelta(days=1))
    result = await NetworkService(NetworkRepository(dataset.connection)).network(target, "shop-a")
    assert result.truncated
    assert len(result.nodes) == 200
    ids = {node.id for node in result.nodes}
    assert all(edge.source in ids and edge.target in ids for edge in result.edges)
    assert len([node for node in result.nodes if node.kind == "checkout"]) == 194
