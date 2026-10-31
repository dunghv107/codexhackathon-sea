from collections.abc import AsyncIterator
from typing import Annotated, Any

from fastapi import Depends, Header, HTTPException, Request
from psycopg import AsyncConnection, IntegrityError, OperationalError
from psycopg_pool import PoolTimeout

from app.db.postgres import Database
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.services.errors import ConflictError, NotFoundError
from app.services.order_service import OrderService
from app.services.scoring_service import ScoringService


async def connection(request: Request) -> AsyncIterator[AsyncConnection[dict[str, Any]]]:
    database: Database | None = getattr(request.app.state, "database", None)
    if database is None:
        raise HTTPException(503, "KBS database is not configured")
    try:
        async with database.pool.connection() as conn:
            yield conn
    except NotFoundError as error:
        raise HTTPException(404, str(error)) from None
    except ConflictError as error:
        raise HTTPException(409, str(error)) from None
    except IntegrityError:
        raise HTTPException(409, "Operation conflicts with stored data") from None
    except (OperationalError, PoolTimeout):
        raise HTTPException(503, "KBS database unavailable") from None


Connection = Annotated[AsyncConnection[dict[str, Any]], Depends(connection, scope="function")]


async def shop_id(x_shop_id: Annotated[str, Header(min_length=1, max_length=200)], conn: Connection) -> str:
    from app.core.kbs_config import SIM_BATCH

    cursor = await conn.execute("SELECT 1 FROM sfd.shops WHERE shop_id = %s AND batch_id = %s", (x_shop_id, SIM_BATCH))
    if await cursor.fetchone() is None:
        raise HTTPException(404, "Shop not found")
    return x_shop_id


Shop = Annotated[str, Depends(shop_id)]


def scoring_service(conn: Connection) -> ScoringService:
    return ScoringService(RiskRepository(conn))


def order_service(conn: Connection) -> OrderService:
    return OrderService(OrderRepository(conn))


Scoring = Annotated[ScoringService, Depends(scoring_service)]
Orders = Annotated[OrderService, Depends(order_service)]
