from typing import Any

from psycopg import AsyncConnection

from app.core.kbs_config import SIM_BATCH
from app.services.errors import NotFoundError


class NetworkRepository:
    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.connection = connection

    async def neighborhood(self, order_id: str, shop_id: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
        cursor = await self.connection.execute(
            "SELECT c.placed_at, i.phone_key, i.device_key, i.address_cluster_id FROM sfd.orders o "
            "JOIN sfd.checkouts c ON c.checkout_id = o.checkout_id AND c.batch_id = o.batch_id "
            "LEFT JOIN sfd.checkout_identity_snapshots i ON i.checkout_id = c.checkout_id "
            "AND i.batch_id = c.batch_id AND i.available_at <= c.placed_at "
            "WHERE o.order_id = %s AND o.shop_id = %s AND o.batch_id = %s",
            (order_id, shop_id, SIM_BATCH),
        )
        target = await cursor.fetchone()
        if target is None:
            raise NotFoundError("Order not found")
        cursor = await self.connection.execute(
            "WITH matched AS ("
            "SELECT c.checkout_id, array_remove(ARRAY["
            "CASE WHEN i.phone_key = %(phone)s THEN 'phone' END, "
            "CASE WHEN i.device_key = %(device)s THEN 'device' END, "
            "CASE WHEN i.address_cluster_id = %(cluster)s THEN 'address' END], NULL) AS links, "
            "EXISTS (SELECT 1 FROM sfd.orders o WHERE o.checkout_id = c.checkout_id "
            "AND o.batch_id = %(batch)s AND o.shop_id = %(shop)s) AS own_shop "
            "FROM sfd.checkouts c JOIN sfd.checkout_identity_snapshots i "
            "ON i.checkout_id = c.checkout_id AND i.batch_id = c.batch_id "
            "WHERE c.batch_id = %(batch)s AND c.placed_at < %(cutoff)s AND i.available_at < %(cutoff)s "
            "AND (i.phone_key = %(phone)s OR i.device_key = %(device)s OR i.address_cluster_id = %(cluster)s)), "
            "own AS (SELECT checkout_id, links FROM matched WHERE own_shop ORDER BY checkout_id LIMIT 194) "
            "SELECT checkout_id, links, 1::bigint AS count FROM own "
            "UNION ALL SELECT NULL, ARRAY[link], count(DISTINCT checkout_id) FROM matched, "
            "unnest(links) AS link WHERE NOT own_shop GROUP BY link ORDER BY checkout_id NULLS LAST, links",
            {
                "batch": SIM_BATCH,
                "shop": shop_id,
                "cutoff": target["placed_at"],
                "phone": target["phone_key"],
                "device": target["device_key"],
                "cluster": target["address_cluster_id"],
            },
        )
        return target, list(await cursor.fetchall())
