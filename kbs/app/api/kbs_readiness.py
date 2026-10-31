from fastapi import FastAPI, HTTPException
from psycopg import Error
from psycopg_pool import PoolTimeout

from app.db.postgres import Database


async def check_database_ready(app: FastAPI) -> None:
    database: Database | None = getattr(app.state, "database", None)
    if database is not None:
        try:
            await database.ready()
        except (Error, PoolTimeout):
            raise HTTPException(503, "KBS database unavailable") from None
