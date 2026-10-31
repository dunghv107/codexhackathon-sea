import asyncio
from collections import OrderedDict
from decimal import Decimal, InvalidOperation

import httpx

from app.agents.explain import ExplainAgent, ExplanationRefusedError
from app.schemas.orders import Explanation
from app.schemas.risk import Assessment


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
        selected = list(reasons)[:3]
        source = "template"
        fallback_reason = "disabled" if reasons else "no_fired_rules"
        if self.agent is not None and reasons:
            evidence: list[dict[str, object]] = []
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
                evidence.append({"rule_id": reason.rule_id, "contribution": reason.contribution, "facts": facts})
            try:
                async with asyncio.timeout(4):
                    selected = await self.agent.select(evidence)
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
        sentences = [
            f"{rule_id}: {reasons[rule_id].description or 'Rule đã kích hoạt'} (+{reasons[rule_id].contribution} điểm)."
            for rule_id in selected
        ] or ["Không có rule nào kích hoạt; dữ liệu thiếu không được coi là bằng chứng an toàn."]
        result = Explanation(assessment_id=key, source=source, sentences=sentences, fallback_reason=fallback_reason)
        # Transient provider failures must remain retryable on the next request.
        if self.agent is None or source == "llm" or not reasons:
            self.cache[key] = result.model_copy(deep=True)
            if len(self.cache) > 256:
                self.cache.popitem(last=False)
        return result
