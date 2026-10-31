from app.repositories.order_repository import OrderRepository
from app.schemas.orders import Explanation, OrderDetail, OrderSummary
from app.schemas.risk import RiskLevel
from app.services.errors import ConflictError
from app.services.explanation_service import ExplanationService


class OrderService:
    def __init__(self, repository: OrderRepository, explainer: ExplanationService) -> None:
        self.repository = repository
        self.explainer = explainer

    async def list_orders(
        self, shop_id: str, risk_level: RiskLevel | None, review_status: str | None, limit: int, offset: int
    ) -> list[OrderSummary]:
        return await self.repository.list_orders(shop_id, risk_level, review_status, limit, offset)

    async def detail(self, order_id: str, shop_id: str) -> OrderDetail:
        return await self.repository.detail(order_id, shop_id)

    async def explain(self, order_id: str, shop_id: str) -> Explanation:
        assessment = (await self.repository.detail(order_id, shop_id)).assessment
        if assessment is None:
            raise ConflictError("Score the order before requesting an explanation")
        return await self.explainer.explain(assessment)
