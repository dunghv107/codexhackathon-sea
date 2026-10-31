import hashlib
from collections import defaultdict
from datetime import datetime, timedelta
from decimal import Decimal

from app.rules.adaptive_config import AdaptiveRuleDefinition, AdaptiveRuleId, BaselineParameters, FeatureName
from app.rules.config import RuleConfig
from app.schemas.adaptive import (
    AdaptiveAssessment,
    AdaptiveContext,
    AdaptiveEvidence,
    BaselineSnapshot,
    Coverage,
    HistoricalOrder,
    ShadowSignal,
    SignalStatus,
)

Sample = tuple[Decimal, datetime, str]


def source_reference(samples: list[Sample]) -> str:
    # Opaque provenance never exposes another shop's order IDs or identity keys.
    raw = "\n".join(sorted(f"{source}|{value}|{at.isoformat()}" for value, at, source in samples))
    return "sha256:" + hashlib.sha256(raw.encode()).hexdigest()


class AdaptiveEvaluator:
    def __init__(self, config: RuleConfig, context: AdaptiveContext, legacy_statuses: dict[str, SignalStatus]) -> None:
        self.config = config
        self.target = context.target
        self.history = context.history
        self.cutoff = self.target.placed_at
        self.statuses = dict(legacy_statuses)
        self.features: dict[str, list[AdaptiveEvidence]] = {}

    def baseline(
        self, samples: list[Sample], cohort: str, params: BaselineParameters, selection: str
    ) -> BaselineSnapshot:
        start = self.cutoff - timedelta(days=params.lookback_days)
        samples = [sample for sample in samples if start <= sample[1] < self.cutoff]
        values = sorted(sample[0] for sample in samples)
        available_at = max((sample[1] for sample in samples), default=None)
        missing = None
        threshold = None
        if len(values) < params.min_samples:
            missing = f"Cần {params.min_samples} mẫu, hiện có {len(values)}"
        elif available_at is None or available_at < self.cutoff - timedelta(days=params.max_age_days):
            missing = "Baseline quá cũ"
        elif values[0] == values[-1]:
            missing = "Phân phối lịch sử không có biến thiên"
        else:
            rank = (len(values) - 1) * Decimal(str(params.quantile))
            lower = int(rank)
            threshold = values[lower] + (values[min(lower + 1, len(values) - 1)] - values[lower]) * (rank - lower)
        return BaselineSnapshot(
            version=self.config.baseline_version or "",
            cohort=cohort,
            sample_count=len(values),
            quantile=Decimal(str(params.quantile)),
            threshold=threshold,
            window_start=start,
            window_end=self.cutoff,
            available_at=available_at,
            selection_policy=selection,
            source_ref=source_reference(samples),
            missing_reason=missing,
        )

    def evidence(
        self,
        feature: str,
        label: str,
        observed: Decimal | None,
        baseline: BaselineSnapshot | None,
        missing: str | None = None,
    ) -> AdaptiveEvidence:
        evidence = AdaptiveEvidence(
            feature=feature,
            label=label,
            observed=observed,
            baseline=baseline,
            missing_reason=missing or (baseline.missing_reason if baseline else None),
        )
        self.features.setdefault(feature, []).append(evidence)
        return evidence

    def history_for(self, params: BaselineParameters) -> list[HistoricalOrder]:
        start = self.cutoff - timedelta(days=params.lookback_days)
        return [row for row in self.history if start <= row.placed_at < self.cutoff]

    @staticmethod
    def buyer_failure(row: HistoricalOrder) -> bool:
        return row.outcome == "delivery_failed" and row.resolution_verified and row.responsible_party == "buyer"

    @staticmethod
    def resolved(row: HistoricalOrder) -> bool:
        return row.outcome == "delivery_succeeded" or (
            row.outcome == "delivery_failed"
            and row.resolution_verified
            and row.responsible_party in {"buyer", "seller", "carrier"}
        )

    def buyer_rate(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        history = [row for row in self.history_for(params) if self.resolved(row)]
        buyer = [row for row in history if row.buyer_id == self.target.buyer_id]
        peers: dict[str, list[HistoricalOrder]] = defaultdict(list)
        for row in history:
            if row.shop_id == self.target.shop_id and row.buyer_id != self.target.buyer_id:
                peers[row.buyer_id].append(row)
        supported = [rows for rows in peers.values() if len(rows) >= params.min_resolved]
        resolved = sum(len(rows) for rows in supported)
        prior = (
            Decimal(sum(self.buyer_failure(row) for rows in supported for row in rows)) / resolved
            if resolved
            else Decimal(0)
        )
        alpha = Decimal(str(params.alpha))
        samples = []
        for rows in supported:
            value = (sum(self.buyer_failure(row) for row in rows) + alpha * prior) / (len(rows) + alpha)
            at = max(
                max(
                    row.outcome_available_at or row.placed_at,
                    (row.resolution_available_at or row.placed_at)
                    if row.outcome == "delivery_failed"
                    else row.placed_at,
                )
                for row in rows
            )
            samples.append((value, at, "|".join(sorted(row.order_id for row in rows))))
        baseline = self.baseline(
            samples, "shop_currency_buyer_rates", params, "successful_or_verified_failed_shop_orders"
        )
        missing = None
        observed = None
        if len(buyer) < params.min_resolved:
            missing = f"Buyer cần {params.min_resolved} kết quả giao đã rõ trách nhiệm, hiện có {len(buyer)}"
        elif not resolved:
            missing = "Không có cohort đủ dữ liệu để tính prior"
        else:
            observed = (sum(self.buyer_failure(row) for row in buyer) + alpha * prior) / (len(buyer) + alpha)
        return [
            self.evidence("buyer_failure_rate", "Tỷ lệ lỗi giao đã xác minh của người mua", observed, baseline, missing)
        ]

    def quantities(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        delivered = [
            row
            for row in self.history_for(params)
            if row.shop_id == self.target.shop_id and row.outcome == "delivery_succeeded"
        ]
        results = []
        for index, (listing, quantity) in enumerate(sorted(self.target.quantities.items()), start=1):
            baseline = None
            for cohort in params.quantity_cohorts:
                samples = [
                    (Decimal(row.quantities[listing]), row.outcome_available_at or row.placed_at, row.order_id)
                    for row in delivered
                    if listing in row.quantities and (cohort == "shop_listing" or row.buyer_id == self.target.buyer_id)
                ]
                baseline = self.baseline(samples, cohort, params, "delivered_shop_orders_per_listing")
                if baseline.missing_reason is None:
                    break
            results.append(
                self.evidence("quantity_per_listing", f"Số lượng mặt hàng {index}", Decimal(quantity), baseline)
            )
        if not results:
            results.append(self.evidence("quantity_per_listing", "Số lượng", None, None, "Không có snapshot giỏ hàng"))
        return results

    def amounts(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        rows = [
            row
            for row in self.history_for(params)
            if row.shop_id == self.target.shop_id and row.outcome == "delivery_succeeded"
        ]
        results = []
        for feature, cohort, buyer_only in (
            ("buyer_order_amount", "buyer_shop_currency", True),
            ("shop_order_amount", "shop_currency", False),
        ):
            samples = [
                (row.amount, row.outcome_available_at or row.placed_at, row.order_id)
                for row in rows
                if not buyer_only or row.buyer_id == self.target.buyer_id
            ]
            baseline = self.baseline(samples, cohort, params, "delivered_shop_orders_same_currency")
            results.append(
                self.evidence(feature, f"Giá trị đơn ({self.target.currency})", self.target.amount, baseline)
            )
        return results

    def velocity(self, params: BaselineParameters, *, duplicate: bool = False) -> list[AdaptiveEvidence]:
        rows = [
            row for row in self.history if row.shop_id == self.target.shop_id and row.buyer_id == self.target.buyer_id
        ]
        feature = "open_duplicate_checkouts" if duplicate else "buyer_shop_velocity"
        label = "Checkout trùng giỏ và địa chỉ đang mở" if duplicate else "Số lần đặt tại cửa hàng"
        if duplicate and not self.target.address_key:
            return [self.evidence(feature, label, None, None, "Thiếu khóa địa chỉ tại thời điểm đặt hàng")]

        def count_at(anchor: HistoricalOrder) -> Decimal:
            start = anchor.placed_at - timedelta(hours=params.window_hours)
            matching = {
                row.checkout_id
                for row in rows
                if start <= row.placed_at < anchor.placed_at
                and (
                    not duplicate
                    or (
                        row.address_key == anchor.address_key
                        and row.quantities == anchor.quantities
                        and (row.terminal_at is None or row.terminal_at >= anchor.placed_at)
                    )
                )
            }
            return Decimal(len(matching) + 1)

        anchors = [
            row
            for row in rows
            if row.placed_at >= self.cutoff - timedelta(days=params.lookback_days)
            and (not duplicate or row.address_key)
        ]
        samples = [(count_at(row), row.placed_at, row.order_id) for row in anchors]
        baseline = self.baseline(
            samples, "buyer_shop_checkout_anchors", params, "checkout_anchored_windows_including_anchor"
        )
        return [self.evidence(feature, label, count_at(self.target), baseline)]

    def contact(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        rows = [row for row in self.history_for(params) if row.buyer_id == self.target.buyer_id and row.phone_key]
        # Count a multi-shop checkout once; do not serialize contact keys.
        contacts = {row.checkout_id: row for row in rows}
        latest = max((row.placed_at for row in contacts.values()), default=None)
        missing = None
        if not self.target.phone_key:
            missing = "Thiếu khóa điện thoại của checkout"
        elif len(contacts) < params.min_samples:
            missing = f"Lịch sử contact cần {params.min_samples} checkout, hiện có {len(contacts)}"
        elif latest is None or latest < self.cutoff - timedelta(days=params.max_age_days):
            missing = "Lịch sử contact quá cũ"
        baseline = BaselineSnapshot(
            version=self.config.baseline_version or "",
            cohort="buyer_phone_history",
            sample_count=len(contacts),
            threshold=Decimal(0),
            window_start=self.cutoff - timedelta(days=params.lookback_days),
            window_end=self.cutoff,
            available_at=latest,
            selection_policy="observed_phone_snapshots_not_identity_verification",
            source_ref=source_reference([(Decimal(1), row.placed_at, row.checkout_id) for row in contacts.values()]),
            missing_reason=missing,
        )
        observed = (
            None if missing else Decimal(int(all(row.phone_key != self.target.phone_key for row in contacts.values())))
        )
        return [self.evidence("buyer_phone_history", "Contact mới so với lịch sử người mua", observed, baseline)]

    def exclusions(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        rows = [
            row
            for row in self.history_for(params)
            if row.buyer_id == self.target.buyer_id
            and row.outcome == "delivery_failed"
            and row.resolution_verified
            and row.responsible_party in {"seller", "carrier"}
        ]
        baseline = BaselineSnapshot(
            version=self.config.baseline_version or "",
            cohort="buyer_order_responsibility",
            sample_count=len(rows),
            threshold=Decimal(0),
            window_start=self.cutoff - timedelta(days=params.lookback_days),
            window_end=self.cutoff,
            available_at=max((row.resolution_available_at or row.placed_at for row in rows), default=None),
            selection_policy="latest_verified_resolution_on_same_shop_order",
            source_ref=source_reference(
                [(Decimal(1), row.resolution_available_at or row.placed_at, row.order_id) for row in rows]
            ),
        )
        return [
            self.evidence(
                "excluded_buyer_failures",
                "Kết quả thuộc trách nhiệm người bán hoặc vận chuyển",
                Decimal(len(rows)),
                baseline,
            )
        ]

    def coverage(self, required: list[FeatureName]) -> Coverage:
        missing: dict[FeatureName, str] = {}
        known: list[FeatureName] = []
        for name in required:
            facts = self.features.get(name, [])
            problems = [
                fact.missing_reason or "Thiếu giá trị quan sát"
                for fact in facts
                if fact.missing_reason or fact.observed is None
            ]
            if not facts or problems:
                missing[name] = "; ".join(dict.fromkeys(problems)) or "Feature chưa được đánh giá"
            else:
                known.append(name)
        return Coverage(
            status="insufficient_data" if missing else "complete",
            required_features=required,
            known_features=known,
            missing_features=missing,
        )

    @staticmethod
    def compare(evidence: list[AdaptiveEvidence], effect: Decimal, *, require_all: bool = False) -> SignalStatus:
        matches: list[bool | None] = []
        for fact in evidence:
            if fact.missing_reason or fact.observed is None or fact.baseline is None or fact.baseline.threshold is None:
                matches.append(None)
            else:
                fact.comparison_threshold = fact.baseline.threshold + effect
                matches.append(fact.observed > fact.comparison_threshold)
        if require_all:
            if False in matches:
                return "clear"
            return "unknown" if None in matches or not matches else "fired"
        if True in matches:
            return "fired"
        return "unknown" if None in matches or not matches else "clear"

    def evaluate(self) -> AdaptiveAssessment:
        definitions = {rule.rule_id: rule for rule in self.config.rules if isinstance(rule, AdaptiveRuleDefinition)}
        signals = []
        groups: dict[str, list[AdaptiveRuleId]] = defaultdict(list)
        required: list[FeatureName] = []
        for rule_id in self.config.dependency_order:
            rule = definitions.get(rule_id)
            if rule is None or not rule.enabled:
                continue
            params = rule.params
            match rule.rule_id:
                case "SFD-A01":
                    evidence = self.buyer_rate(params)
                case "SFD-A04":
                    evidence = self.velocity(params)
                case "SFD-A06":
                    evidence = self.velocity(params, duplicate=True)
                case "SFD-A08":
                    evidence = self.quantities(params)
                case "SFD-A09":
                    evidence = self.amounts(params)
                case "SFD-A11":
                    evidence = self.contact(params)
                case "SFD-A18":
                    evidence = self.exclusions(params)
                case "SFD-A17":
                    required = params.required_features
                    evidence = []
            status = self.compare(evidence, Decimal(str(params.minimum_effect)), require_all=rule.rule_id == "SFD-A09")
            missing = "; ".join(dict.fromkeys(fact.missing_reason for fact in evidence if fact.missing_reason)) or None
            if rule.rule_id == "SFD-A17":
                status = "fired" if self.coverage(required).missing_features else "clear"
            if rule.dependencies and status != "clear":
                states = [self.statuses.get(parent, "unknown") for parent in rule.dependencies.rules]
                if rule.dependencies.mode == "any":
                    dependency_status = (
                        "fired" if "fired" in states else ("unknown" if "unknown" in states else "clear")
                    )
                else:
                    dependency_status = (
                        "clear" if "clear" in states else ("unknown" if "unknown" in states else "fired")
                    )
                if dependency_status != "fired":
                    status = dependency_status
                    if status == "unknown":
                        missing = "Chưa đủ dữ liệu của tín hiệu kết hợp"
            self.statuses[rule.rule_id] = status
            signals.append(
                ShadowSignal(
                    rule_id=rule.rule_id,
                    status=status,
                    description=rule.description,
                    action=rule.action,
                    evidence_group=rule.evidence_group,
                    evidence=evidence,
                    missing_reason=missing if status == "unknown" else None,
                )
            )
            if status == "fired":
                groups[rule.evidence_group].append(rule.rule_id)
        return AdaptiveAssessment(
            baseline_version=self.config.baseline_version or "",
            policy_version=self.config.version,
            input_cutoff_at=self.cutoff,
            signals=signals,
            coverage=self.coverage(required),
            evidence_groups=dict(groups),
        )
