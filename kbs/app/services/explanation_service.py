import asyncio
from collections import OrderedDict
from decimal import Decimal, InvalidOperation

import httpx

from app.agents.explain import ExplainAgent, ExplanationRefusedError
from app.schemas.orders import Explanation
from app.schemas.risk import Assessment

RULE_MEANINGS = {
    "R1": "Lịch sử giao thất bại đã xác minh do người mua, theo số điện thoại",
    "R2": "Lịch sử giao thất bại đã xác minh do người mua, theo cụm địa chỉ",
    "R3": "Giao thất bại lặp lại đã xác minh do người mua, theo số điện thoại",
    "R4": "Nhiều lần đặt hàng riêng biệt liên quan đến nhiều cửa hàng",
    "R5": "Nhiều lần đặt hàng trong thời gian ngắn, kèm tín hiệu rủi ro khác",
    "R6": "Nhiều người mua dùng chung cụm địa chỉ có lịch sử giao thất bại",
    "R7": "Giá trị thanh toán khi nhận hàng cao hơn lịch sử cùng cửa hàng",
    "R8": "Tài khoản mới kèm tín hiệu đặt hàng hoặc giá trị đơn bất thường",
    "R9": "Địa chỉ cần kiểm tra",
    "R10": "Người mua từng hủy đơn sau khi xác nhận",
}


def template_sentence(rule_id: str, facts: dict[str, str]) -> str:
    """Provide useful evidence even when no provider is available."""
    if rule_id in {"R1", "R2"} and "resolved" in facts and "failed" in facts:
        identity = "số điện thoại" if rule_id == "R1" else "cụm địa chỉ"
        return (
            f"Trong {facts['resolved']} lần mua có kết quả giao liên quan đến {identity}, "
            f"{facts['failed']} lần thất bại đã được xác minh do người mua. "
            "Lịch sử này cần được đối chiếu thêm; hãy hỏi lại khách về nhu cầu nhận hàng và địa chỉ giao."
        )
    if rule_id in {"R3", "R10"} and "count" in facts:
        event = "giao thất bại đã xác minh do người mua" if rule_id == "R3" else "người mua hủy sau xác nhận"
        return (
            f"Lịch sử đã xét ghi nhận {facts['count']} lần {event}. "
            "Điều này chưa kết luận đơn hiện tại sẽ thất bại; hãy xác nhận lại khả năng nhận hàng với khách."
        )
    return f"{RULE_MEANINGS[rule_id]}. Hãy đối chiếu dữ kiện của tín hiệu này và xác nhận thông tin đơn với khách."


class ExplanationService:
    def __init__(self, agent: ExplainAgent | None = None) -> None:
        self.agent = agent
        # shortcut: bounded per-process cache; use shared storage if multi-worker reuse becomes necessary.
        self.cache: OrderedDict[str, Explanation] = OrderedDict()

    async def explain(self, assessment: Assessment) -> Explanation:
        key = assessment.assessment_id
        if key in self.cache:
            self.cache.move_to_end(key)
            return self.cache[key].model_copy(deep=True)
        reasons = {reason.rule_id: reason for reason in assessment.reasons}
        evidence: list[dict[str, object]] = []
        sentences: list[str] = []
        source = "template"
        fallback_reason = "disabled" if reasons else "no_fired_rules"
        for reason in reasons.values():
            facts: dict[str, str] = {}
            snapshot = assessment.feature_snapshot.get(reason.rule_id)
            if isinstance(snapshot, dict):
                for name, value in snapshot.items():
                    if name not in {
                        "resolved",
                        "failed",
                        "count",
                        "checkouts",
                        "shops",
                        "buyers",
                        "amount",
                        "median",
                        "history_orders",
                        "age_days",
                        "cod",
                        "smoothed_rate",
                        "failure_rate",
                        "ratio",
                    }:
                        continue
                    # Only finite aggregate numbers leave the service, never identity keys or free text.
                    try:
                        number = Decimal(str(value))
                        if number.is_finite():
                            facts[str(name)] = str(number)
                    except InvalidOperation:
                        continue
            evidence.append(
                {
                    "rule_id": reason.rule_id,
                    "meaning": RULE_MEANINGS[reason.rule_id],
                    "contribution": reason.contribution,
                    "facts": facts,
                }
            )
            sentences.append(template_sentence(reason.rule_id, facts))
        sentences = sentences[:3] or [
            "Chưa có tín hiệu làm tăng điểm trong lịch sử đã biết. "
            "Thiếu dữ liệu không phải bằng chứng an toàn; hãy kiểm tra thông tin nhận hàng trước khi xử lý."
        ]
        if self.agent is not None and reasons:
            try:
                async with asyncio.timeout(15):
                    explanations = await self.agent.explain(evidence)
                sentences = [item.explanation for item in explanations]
                source = "llm"
                fallback_reason = None
            except ExplanationRefusedError:
                fallback_reason = "provider_refusal"
            except (httpx.TimeoutException, TimeoutError):
                fallback_reason = "provider_timeout"
            except httpx.HTTPError:
                fallback_reason = "provider_unavailable"
            except (ValueError, TypeError, KeyError, AttributeError):
                fallback_reason = "invalid_output"
        result = Explanation(assessment_id=key, source=source, sentences=sentences, fallback_reason=fallback_reason)
        # Transient provider failures must remain retryable on the next request.
        if self.agent is None or source == "llm" or not reasons:
            self.cache[key] = result.model_copy(deep=True)
            if len(self.cache) > 256:
                self.cache.popitem(last=False)
        return result
