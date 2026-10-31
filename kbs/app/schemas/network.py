from datetime import datetime
from typing import Literal

from pydantic import BaseModel


class NetworkNode(BaseModel):
    id: str
    kind: Literal["checkout", "identity", "aggregate"]
    label: str
    count: int = 1


class NetworkEdge(BaseModel):
    source: str
    target: str


class OrderNetwork(BaseModel):
    order_id: str
    input_cutoff_at: datetime
    nodes: list[NetworkNode]
    edges: list[NetworkEdge]
    truncated: bool
