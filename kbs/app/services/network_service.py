from app.repositories.network_repository import NetworkRepository
from app.schemas.network import NetworkEdge, NetworkNode, OrderNetwork


class NetworkService:
    def __init__(self, repository: NetworkRepository) -> None:
        self.repository = repository

    async def network(self, order_id: str, shop_id: str) -> OrderNetwork:
        target, rows = await self.repository.neighborhood(order_id, shop_id)
        nodes = [NetworkNode(id="target", kind="checkout", label="Checkout đang xem")]
        edges = []
        for column, key, label in (
            ("phone_key", "phone", "Cùng điện thoại"),
            ("device_key", "device", "Cùng thiết bị"),
            ("address_cluster_id", "address", "Cùng cụm địa chỉ"),
        ):
            if target[column] is not None:
                nodes.append(NetworkNode(id=key, kind="identity", label=label))
                edges.append(NetworkEdge(source="target", target=key))
        own = [row for row in rows if row["checkout_id"] is not None]
        # Reserve seven nodes for the target, three identities and three cross-shop aggregates.
        for index, row in enumerate(own[:193]):
            node_id = f"history:{index}"
            nodes.append(NetworkNode(id=node_id, kind="checkout", label="Checkout trước đó tại shop"))
            edges.extend(NetworkEdge(source=key, target=node_id) for key in row["links"])
        for row in rows:
            if row["checkout_id"] is None:
                key = row["links"][0]
                node_id = f"other:{key}"
                nodes.append(NetworkNode(id=node_id, kind="aggregate", label="Checkout shop khác", count=row["count"]))
                edges.append(NetworkEdge(source=key, target=node_id))
        return OrderNetwork(
            order_id=order_id,
            input_cutoff_at=target["placed_at"],
            nodes=nodes,
            edges=edges,
            truncated=len(own) > 193,
        )
