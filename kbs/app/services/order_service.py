from app.repositories.order_repository import OrderRepository
from app.schemas.orders import DecisionRequest, Explanation, OrderDetail, OrderSummary, ReviewDecision
from app.schemas.risk import RiskLevel
from app.services.errors import ConflictError


class OrderService:
    def __init__(self, repository: OrderRepository) -> None:
        self.repository = repository

    async def list_orders(
        self, shop_id: str, risk_level: RiskLevel | None, review_status: str | None, limit: int, offset: int
    ) -> list[OrderSummary]:
        return await self.repository.list_orders(shop_id, risk_level, review_status, limit, offset)

    async def detail(self, order_id: str, shop_id: str) -> OrderDetail:
        return await self.repository.detail(order_id, shop_id)

    async def decide(self, order_id: str, shop_id: str, request: DecisionRequest) -> ReviewDecision:
        return await self.repository.decide(order_id, shop_id, request)

    async def explain(self, order_id: str, shop_id: str) -> Explanation:
        assessment = (await self.repository.detail(order_id, shop_id)).assessment
        if assessment is None:
            raise ConflictError("Score the order before requesting an explanation")
        # shortcut: deterministic templates only; add a validated provider when agent work is in scope.
        sentences = [
            f"{reason.rule_id}: {reason.description} (+{reason.points} điểm)." for reason in assessment.reasons[:3]
        ]
        if not sentences:
            sentences = ["Không có rule nào kích hoạt; dữ liệu thiếu không được coi là bằng chứng an toàn."]
        return Explanation(assessment_id=assessment.assessment_id, sentences=sentences)
