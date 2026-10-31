from typing import Any

from psycopg import AsyncConnection
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool


class Database:
    def __init__(self, url: str) -> None:
        self.pool = AsyncConnectionPool[AsyncConnection[dict[str, Any]]](
            url,
            open=False,
            min_size=1,
            max_size=8,
            timeout=10,
            kwargs={"row_factory": dict_row, "options": "-c search_path=sfd,public -c statement_timeout=30000"},
        )

    async def open(self) -> None:
        await self.pool.open()
        try:
            await self.pool.wait(timeout=10)
            async with self.pool.connection() as connection:
                await connection.execute("SELECT 1 FROM sfd.data_batches LIMIT 1")
        except Exception:
            await self.pool.close()
            raise

    async def close(self) -> None:
        await self.pool.close()

    async def ready(self) -> None:
        async with self.pool.connection() as connection:
            await connection.execute("SELECT 1 FROM sfd.data_batches LIMIT 1")
