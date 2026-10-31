import hashlib
from collections import defaultdict
from datetime import datetime, timedelta
from decimal import Decimal

from app.rules.adaptive_config import (
    AdaptiveRuleDefinition,
    AdaptiveRuleId,
    BaselineParameters,
    ExtendedParameters,
    FeatureName,
)
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
        self.context = context
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

    def predicate(
        self,
        feature: str,
        label: str,
        observed: Decimal | None,
        threshold: Decimal,
        params: BaselineParameters,
        cohort: str,
        samples: list[Sample],
        missing: str | None = None,
    ) -> AdaptiveEvidence:
        baseline = BaselineSnapshot(
            version=self.config.baseline_version or "",
            cohort=cohort,
            sample_count=len(samples),
            threshold=threshold,
            window_start=self.cutoff - timedelta(days=params.lookback_days),
            window_end=self.cutoff,
            available_at=max((s[1] for s in samples), default=None),
            selection_policy=cohort,
            source_ref=source_reference(samples),
            missing_reason=missing,
        )
        return self.evidence(feature, label, observed, baseline, missing)

    @staticmethod
    def cancellation_bad(row: HistoricalOrder) -> bool:
        return row.cancellation_verified and row.cancellation_at is not None and row.cancellation_party == "buyer"

    @staticmethod
    def after_sale_bad(row: HistoricalOrder) -> bool:
        return (
            row.after_sale_verified
            and row.after_sale_party == "buyer"
            and row.after_sale_code in {"return_abuse", "refund_abuse"}
        )

    def outcome_rate(
        self, params: BaselineParameters, policy: ExtendedParameters, *, after_sale: bool
    ) -> list[AdaptiveEvidence]:
        rows = self.history_for(params)
        if after_sale:
            rows = [
                r
                for r in rows
                if r.outcome == "delivery_succeeded"
                and r.outcome_available_at is not None
                and r.outcome_available_at <= self.cutoff - timedelta(days=policy.maturity_days)
                and (not r.after_sale_present or r.after_sale_verified)
            ]
            bad = self.after_sale_bad
            feature, label = "after_sale_rate", "Tỷ lệ lạm dụng hoàn trả đã xác minh"
        else:
            rows = [
                r
                for r in rows
                if r.confirmed_at is not None
                and (
                    r.outcome == "delivery_succeeded"
                    or (
                        r.cancellation_at is not None
                        and r.cancellation_verified
                        and r.cancellation_party in {"buyer", "seller", "carrier"}
                    )
                )
            ]
            bad = self.cancellation_bad
            feature, label = "buyer_cancellation_rate", "Tỷ lệ hủy sau xác nhận đã rõ trách nhiệm"
        # Category snapshots are used only when supported; fallback preserves shop-order/currency units.
        cohorts = [("shop_currency_resolved_buyers", rows)]
        if after_sale and self.target.categories:
            cohorts.insert(
                0,
                (
                    "shop_category_currency_resolved_buyers",
                    [r for r in rows if r.categories and r.categories & self.target.categories],
                ),
            )
        selected: AdaptiveEvidence | None = None
        for cohort, candidates in cohorts:
            peers: dict[str, list[HistoricalOrder]] = defaultdict(list)
            for row in candidates:
                if row.shop_id == self.target.shop_id and row.buyer_id != self.target.buyer_id:
                    peers[row.buyer_id].append(row)
            supported = [r for r in peers.values() if len(r) >= params.min_resolved]
            total = sum(len(r) for r in supported)
            prior = Decimal(sum(bad(r) for group in supported for r in group)) / total if total else Decimal(0)
            alpha = Decimal(str(params.alpha))
            samples = [
                (
                    (sum(bad(r) for r in group) + alpha * prior) / (len(group) + alpha),
                    max(
                        max(
                            r.outcome_available_at or r.placed_at,
                            (r.after_sale_known_at if after_sale else r.cancellation_known_at) or r.placed_at,
                        )
                        for r in group
                    ),
                    "|".join(sorted(r.order_id for r in group)),
                )
                for group in supported
            ]
            baseline = self.baseline(
                samples,
                cohort,
                params,
                "mature_delivered_and_verified_case_outcomes"
                if after_sale
                else "confirmed_successes_or_verified_cancellations",
            )
            buyer = [r for r in candidates if r.buyer_id == self.target.buyer_id]
            missing = None
            observed = None
            if len(buyer) < params.min_resolved or not total:
                missing = "Chưa đủ kết quả đã rõ trách nhiệm của người mua hoặc cohort"
            else:
                observed = (sum(bad(r) for r in buyer) + alpha * prior) / (len(buyer) + alpha)
            selected = AdaptiveEvidence(
                feature=feature,
                label=label,
                observed=observed,
                baseline=baseline,
                observed_sample_count=len(buyer),
                missing_reason=missing or baseline.missing_reason,
            )
            if selected.missing_reason is None:
                break
        if selected is None:
            raise ValueError("Outcome rate requires a cohort")
        self.features.setdefault(feature, []).append(selected)
        return [selected]

    def outcome_change(
        self, params: BaselineParameters, policy: ExtendedParameters, *, seller: bool = False
    ) -> list[AdaptiveEvidence]:
        end = self.cutoff - timedelta(days=policy.maturity_days)
        split = end - timedelta(days=policy.recent_days)
        rows = [
            r
            for r in self.history_for(params)
            if self.resolved(r)
            and (r.shop_id == self.target.shop_id if seller else r.buyer_id == self.target.buyer_id)
            and r.placed_at < end
            and (r.outcome_available_at or r.placed_at) <= end
        ]
        recent = [r for r in rows if r.placed_at >= split]
        reference = [r for r in rows if r.placed_at < split]
        bad = (lambda r: r.seller_case) if seller else self.buyer_failure
        prior = Decimal(sum(bad(r) for r in reference)) / len(reference) if reference else Decimal(0)
        alpha = Decimal(str(params.alpha))
        observed = (sum(bad(r) for r in recent) + alpha * prior) / (len(recent) + alpha)
        samples = [
            (
                Decimal(int(bad(r))),
                max(
                    r.outcome_available_at or r.placed_at,
                    (r.seller_case_known_at if seller else r.resolution_available_at) or r.placed_at,
                ),
                r.order_id,
            )
            for r in reference
        ]
        missing = None
        if len(recent) < params.min_resolved or len(reference) < params.min_resolved:
            missing = "Hai cửa sổ gần đây và tham chiếu đều cần đủ kết quả đã xác minh"
        elif max(s[1] for s in samples) < self.cutoff - timedelta(days=params.max_age_days):
            missing = "Cửa sổ tham chiếu quá cũ"
        feature = "seller_case_change" if seller else "buyer_outcome_change"
        label = "Tỷ lệ hồ sơ xác minh lỗi người bán tăng" if seller else "Tỷ lệ lỗi giao của người mua tăng gần đây"
        fact = self.predicate(
            feature,
            label,
            observed,
            prior,
            params,
            "shop_currency_recent_vs_reference" if seller else "buyer_recent_vs_reference",
            samples,
            missing,
        )
        fact.observed_sample_count = len(recent)
        if fact.baseline:
            fact.baseline.window_end = split
        return [fact]

    def exposure(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        feature, label = "cod_exposure", f"Tổng COD còn mở tại cửa hàng ({self.target.currency})"
        if self.target.payment_method != "cod":
            return [self.predicate(feature, label, Decimal(0), Decimal(0), params, "non_cod_order", [])]
        rows = [
            r
            for r in self.history
            if r.buyer_id == self.target.buyer_id and r.shop_id == self.target.shop_id and r.payment_method == "cod"
        ]

        def at(anchor: HistoricalOrder) -> Decimal | None:
            active = [
                r
                for r in rows
                if r.placed_at < anchor.placed_at and (r.cod_closed_at is None or r.cod_closed_at >= anchor.placed_at)
            ] + [anchor]
            if any(r.tracking_from is None or r.tracking_from > r.placed_at for r in active):
                return None
            return sum((r.amount for r in active), Decimal(0))

        samples = []
        incomplete = False
        for row in rows:
            if row.placed_at < self.cutoff - timedelta(days=params.lookback_days):
                continue
            value = at(row)
            if value is None:
                incomplete = True
            else:
                samples.append((value, row.placed_at, row.order_id))
        observed = at(self.target)
        baseline = self.baseline(
            samples, "buyer_shop_currency_open_cod", params, "checkout_anchors_until_success_cancel_or_return_v1"
        )
        missing = (
            "Thiếu lịch sử đầy đủ để dựng nghĩa vụ COD tại các thời điểm so sánh"
            if incomplete or observed is None
            else None
        )
        return [self.evidence(feature, label, observed, baseline, missing)]

    def shop_expansion(self, params: BaselineParameters, policy: ExtendedParameters) -> list[AdaptiveEvidence]:
        rows = [r for r in self.history if r.buyer_id == self.target.buyer_id]

        def breadth(anchor: HistoricalOrder, current: bool = False) -> tuple[int, int]:
            start = anchor.placed_at - timedelta(hours=params.window_hours)
            nearby = [r for r in rows if start <= r.placed_at <= anchor.placed_at]
            shops = {r.shop_id for r in nearby} | (self.context.checkout_shops if current else {anchor.shop_id})
            return len(shops | {anchor.shop_id}), len({r.checkout_id for r in nearby} | {anchor.checkout_id})

        anchors = {r.checkout_id: r for r in self.history_for(params) if r.buyer_id == self.target.buyer_id}
        samples = [(Decimal(breadth(r)[0]), r.placed_at, r.checkout_id) for r in anchors.values()]
        shops, checkouts = breadth(self.target, True)
        return [
            self.evidence(
                "buyer_shop_expansion",
                "Số cửa hàng trong cửa sổ mua",
                Decimal(shops),
                self.baseline(
                    samples, "buyer_checkout_shop_breadth", params, "distinct_checkouts_including_split_shop_baskets"
                ),
            ),
            self.predicate(
                "expansion_checkouts",
                "Số lần mua riêng biệt",
                Decimal(checkouts),
                Decimal(policy.min_distinct_checkouts - 1),
                params,
                "multiple_checkouts_required",
                samples,
            ),
        ]

    def categories(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        rows = [
            r
            for r in self.history_for(params)
            if r.buyer_id == self.target.buyer_id and r.outcome == "delivery_succeeded"
        ]
        checkouts = {r.checkout_id for r in rows}
        missing = None
        if not self.target.categories or any(r.categories is None for r in rows):
            missing = "Thiếu snapshot danh mục tại thời điểm mua; không dùng danh mục hiện tại để suy ngược"
        elif len(checkouts) < params.min_samples:
            missing = "Chưa đủ lịch sử danh mục đã giao thành công của người mua"
        known = {category for r in rows for category in r.categories or set()}
        observed = Decimal(len(self.target.categories - known)) if self.target.categories is not None else None
        samples = [(Decimal(1), r.outcome_available_at or r.placed_at, r.order_id) for r in rows]
        return [
            self.predicate(
                "new_categories",
                "Nhóm ngành hàng mới so với lịch sử người mua",
                observed,
                Decimal(0),
                params,
                "buyer_delivered_category_snapshots",
                samples,
                missing,
            )
        ]

    def trusted_buyers(self, policy: ExtendedParameters) -> set[str]:
        links = defaultdict(list)
        for link in self.context.identity_links:
            if link.edge_type in policy.allowed_edge_types and link.occurred_at >= self.cutoff - timedelta(
                days=policy.max_link_age_days
            ):
                links[link.related_buyer_id].append(link)
        return {
            buyer
            for buyer, evidence in links.items()
            if len({e.edge_type for e in evidence}) >= policy.min_link_types
            and len({e.source_ref for e in evidence}) >= policy.min_link_sources
        }

    def linked_group(self, params: BaselineParameters, policy: ExtendedParameters) -> list[AdaptiveEvidence]:
        buyers = self.trusted_buyers(policy)
        rows = [r for r in self.history_for(params) if r.buyer_id in buyers]
        bad = {r.buyer_id for r in rows if self.buyer_failure(r) or self.cancellation_bad(r) or self.after_sale_bad(r)}
        synchronized = {r.buyer_id for r in rows if r.placed_at >= self.cutoff - timedelta(hours=params.window_hours)}
        samples = [(Decimal(1), r.placed_at, r.order_id) for r in rows]
        missing = "Chưa có liên kết danh tính đủ loại và nguồn bằng chứng độc lập" if not buyers else None
        return [
            self.predicate(
                "linked_bad_buyers",
                "Tài khoản liên kết có kết quả xấu đã xác minh",
                Decimal(len(bad)),
                Decimal(policy.min_bad_peers - 1),
                params,
                "verified_independent_identity_links",
                samples,
                missing,
            ),
            self.predicate(
                "synchronized_buyers",
                "Tài khoản liên kết cùng đặt trong cửa sổ",
                Decimal(len(synchronized)),
                Decimal(policy.min_synchronized_peers - 1),
                params,
                "verified_linked_checkout_window",
                samples,
                missing,
            ),
        ]

    def recurrence(self, params: BaselineParameters, policy: ExtendedParameters) -> list[AdaptiveEvidence]:
        buyers = self.trusted_buyers(policy)
        created = self.target.account_created_at
        rows = [r for r in self.history_for(params) if r.buyer_id == self.target.buyer_id]
        matched = []
        for enforcement in self.context.enforcements:
            if (
                enforcement.buyer_id not in buyers
                or enforcement.action != "restricted"
                or created is None
                or created <= enforcement.occurred_at
            ):
                continue
            for row in rows:
                repeats = {
                    "delivery": self.buyer_failure(row),
                    "cancellation": self.cancellation_bad(row),
                    "after_sale": self.after_sale_bad(row),
                }
                if row.placed_at >= created and repeats[enforcement.abuse_code]:
                    matched.append(row)
        missing = None
        if created is None or created > self.cutoff:
            missing = "Thiếu thời điểm tạo tài khoản hợp lệ"
        elif not buyers:
            missing = "Chưa có liên kết danh tính đủ bằng chứng độc lập"
        samples = [(Decimal(1), r.placed_at, r.order_id) for r in matched]
        return [
            self.predicate(
                "verified_recurrence",
                "Tài khoản mới lặp hành vi sau xử lý vi phạm",
                Decimal(len({r.order_id for r in matched})),
                Decimal(0),
                params,
                "active_enforcement_and_matching_verified_outcomes",
                samples,
                missing,
            )
        ]

    def evidence_reuse(self, params: BaselineParameters) -> list[AdaptiveEvidence]:
        reviews = self.context.evidence_reviews
        matches = [r for r in reviews if r.verdict == "contradiction_verified" and r.independent_cases > 0]
        missing = None
        if not matches and (
            not reviews or len(reviews) < self.context.evidence_count or any(r.verdict == "unresolved" for r in reviews)
        ):
            missing = "Bằng chứng chưa được băm nội dung và đối chiếu mâu thuẫn đầy đủ"
        samples = [(Decimal(r.independent_cases), r.available_at, r.evidence_id) for r in reviews]
        return [
            self.predicate(
                "evidence_reuse",
                "Bằng chứng trùng nội dung trong hồ sơ độc lập có mâu thuẫn đã xác minh",
                Decimal(len(matches)),
                Decimal(0),
                params,
                "content_hash_and_verified_contradiction",
                samples,
                missing,
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
            policy = rule.policy
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
                case (
                    "SFD-A02"
                    | "SFD-A03"
                    | "SFD-A05"
                    | "SFD-A07"
                    | "SFD-A10"
                    | "SFD-A12"
                    | "SFD-A13"
                    | "SFD-A14"
                    | "SFD-A15"
                    | "SFD-A16"
                ):
                    if policy is None:
                        raise ValueError("Extended rule policy is required")
                    match rule.rule_id:
                        case "SFD-A02" | "SFD-A14":
                            evidence = self.outcome_rate(params, policy, after_sale=rule.rule_id == "SFD-A14")
                        case "SFD-A03" | "SFD-A16":
                            evidence = self.outcome_change(params, policy, seller=rule.rule_id == "SFD-A16")
                        case "SFD-A05":
                            evidence = self.exposure(params)
                        case "SFD-A07":
                            evidence = self.shop_expansion(params, policy)
                        case "SFD-A10":
                            evidence = self.categories(params)
                        case "SFD-A12":
                            evidence = self.linked_group(params, policy)
                        case "SFD-A13":
                            evidence = self.recurrence(params, policy)
                        case "SFD-A15":
                            evidence = self.evidence_reuse(params)
                case "SFD-A17":
                    required = params.required_features
                    evidence = []
            status = self.compare(
                evidence,
                Decimal(str(params.minimum_effect)),
                require_all=rule.rule_id in {"SFD-A07", "SFD-A09", "SFD-A12"},
            )
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
                    audience=rule.audience,
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
