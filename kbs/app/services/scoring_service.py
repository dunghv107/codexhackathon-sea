from app.repositories.risk_repository import RiskRepository
from app.rules.adaptive_config import AdaptiveRuleDefinition
from app.rules.engine import RuleEngine
from app.schemas.adaptive import AdaptiveAssessment
from app.schemas.risk import Assessment
from app.services.errors import ConflictError


class ScoringService:
    def __init__(self, repository: RiskRepository) -> None:
        self.repository = repository

    async def score(self, order_ids: list[str], shop_id: str, *, force: bool = False) -> list[Assessment]:
        order_ids = list(dict.fromkeys(order_ids))
        if not order_ids:
            return []
        await self.repository.lock_orders(order_ids, shop_id)
        config = await self.repository.configuration()
        include_platform = any(
            isinstance(r, AdaptiveRuleDefinition) and r.audience == "platform" and r.enabled for r in config.rules
        )
        previous = await self.repository.previous(order_ids, shop_id, config.version, include_platform=include_platform)
        pending = order_ids if force else [order_id for order_id in order_ids if order_id not in previous]
        results = dict(previous)
        if not force:
            for assessment in previous.values():
                if assessment.risk_level == "low":
                    await self.repository.apply_review_policy(assessment, shop_id)
        if pending:
            engine = RuleEngine(config)
            observed = await self.repository.facts(pending, shop_id, config)
            if {facts.order_id for facts in observed} != set(pending):
                raise ConflictError("Order data is incomplete within the simulation batch")
            for facts in observed:
                assessment = await self.repository.save(
                    facts, config, engine.evaluate(facts.features, facts.adaptive_context)
                )
                if facts.order_id in previous:
                    old = previous[facts.order_id]
                    old_shadow = old.feature_snapshot.get("adaptive")
                    new_shadow = assessment.feature_snapshot.get("adaptive")
                    same_shadow = (
                        AdaptiveAssessment.model_validate(old_shadow) == AdaptiveAssessment.model_validate(new_shadow)
                        if old_shadow is not None and new_shadow is not None
                        else old_shadow == new_shadow
                    )
                    assessment.matches_previous = (
                        old.score == assessment.score
                        and old.risk_level == assessment.risk_level
                        and {(r.rule_id, r.points) for r in old.reasons}
                        == {(r.rule_id, r.points) for r in assessment.reasons}
                        and same_shadow
                        and old.platform_signals == assessment.platform_signals
                    )
                results[facts.order_id] = assessment
        return [results[order_id] for order_id in order_ids]
