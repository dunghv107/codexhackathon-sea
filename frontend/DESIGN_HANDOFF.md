# afd. design handoff

Source: [Claude Design project](https://claude.ai/design/p/dcac756c-fd0e-4ce0-a668-1a8df111b818?file=Portal+Screens.dc.html).

Imported through the authenticated Claude Design MCP from:

- `design_handoff_olist_fraud_review/Portal Screens.dc.html`
- `design_handoff_olist_fraud_review/README.md`
- `design_handoff_olist_fraud_review/Seller Review Prototype v2.dc.html`

The project bundle export failed its thumbnail byte-count verification. The three selected files were instead read in full through MCP; the fonts below were downloaded individually with byte-count/hash verification. No signed preview URLs or credentials are stored here.

## Implementation

The existing React routes, API client and Radix components are retained. The cream/espresso/marigold palette, Be Vietnam Pro typography, sidebar, cards, badges, product placeholders and responsive layouts are adapted from the selected designs. Seller detail uses layout A (three summary tiles) and modal decisions, without the prototype's variant switcher. All production data comes from the existing API.

- Seller: shop switcher, dashboard, review queue with exact-ID lookup, detail tabs, stored risk explanations, decision history, disputes, fulfillment actions, shop management and product forms.
- Buyer: split login, portal choice, catalog, shop catalog, grouped cart, address selection/management, order cards and detail progress.
- Shared: loading skeletons, retry errors, dialog focus management, blocked dismissal during writes, mobile navigation, local font assets.
- Conflict handling retains the reason, refreshes the assessment and blocks resubmission while refresh is pending or failed. A second submission requires an explicit user click.

## API-dependent differences

- Dashboard Hold orders use the existing newest-first API order, labeled accordingly; no unsupported risk sort is implied.
- No fabricated filter counts, buyer status facets, payment status in summary rows or shop order totals. Product illustrations are described below.
- Only the available local Be Vietnam Pro fonts were imported. IDs and JSON use the system monospace fallback.
- Backend validation remains authoritative. Existing KBS score/explain actions from dev are retained in the risk tab; rule contributions accept both `contribution` and `points`. No scoring algorithm, schema, seed or KBS service changes.
- New listings default to VND in the form and API; existing BRL records retain their currency.

## Validation

`npm run build`, `npm run lint`, and `npm test` in `frontend`.

Playwright was run externally against Vite at 1440×1000 and 390×844, using intercepted API responses so that user data was not changed. Checks covered all main routes, horizontal overflow, mobile navigation, detail tabs, buyer risk isolation and a decision conflict retaining its reason and using the refreshed assessment ID. Screenshots were inspected against the MCP seller preview. Live backend integration was not tested because port 8000 was unavailable.

## Imported font assets

Source directory: `ds/fonts/` in the Claude Design project. Files are served locally from `public/fonts/`; no runtime dependency on Claude Design.

| Asset | Bytes | SHA-256 |
| --- | ---: | --- |
| `BeVietnamPro-latin-400.woff2` | 21168 | `03d1b589cff172e1a670b3573e731d3380bc326f80cf83b0d3504e3188e2e074` |
| `BeVietnamPro-latin-500.woff2` | 21892 | `b621f77d35f777023aa11ca524462d511b4b28a813adbc0e9d15a10fc61dfe4e` |
| `BeVietnamPro-latin-600.woff2` | 22032 | `9503dec2a7c532c8331e9600bcafea287a4fd208573b8668d85ab8d8de1863c7` |
| `BeVietnamPro-latin-700.woff2` | 22152 | `a193dd87699bd2e18ddf72dc271493ea82a23dad9f5c334d9f2a257b1e05fc30` |
| `BeVietnamPro-latin-ext-400.woff2` | 13056 | `f7a2811e471c2973a1179ce39da3ad6bb8082381aa8d1535ccfcbc3d6d78a052` |
| `BeVietnamPro-latin-ext-500.woff2` | 13548 | `947d592eb7676914507ff68fc11fcf2cfdecd6ebaf0fa2af6047f66a63ea7d0f` |
| `BeVietnamPro-latin-ext-600.woff2` | 13608 | `20cc75daeaaaa0fa8af2b616b2de2b2900f0ce14ae8e9faa47d272cedd93827f` |
| `BeVietnamPro-latin-ext-700.woff2` | 13852 | `33d57e5bd840b03921568e08d2be1082d453e55f1ba55f421e41a1aa54e12601` |
| `BeVietnamPro-vietnamese-400.woff2` | 11532 | `dc085e2fba3414e5c5bf1e6172f921a9f81c5859946a4ed3d63c1e470d96a9e2` |
| `BeVietnamPro-vietnamese-500.woff2` | 12172 | `86341610cbe907eecf461c9159c168d5efb52bb1a33963813a08f520555d8e66` |
| `BeVietnamPro-vietnamese-600.woff2` | 12176 | `97658c6f9a384f29a3005c3d96e2a0d1c810192cf68979071c290f5a377a9f99` |
| `BeVietnamPro-vietnamese-700.woff2` | 12468 | `4f58af2d1c3e28a9ba14c51c82db2751d78344b75bdcb34de24a1031ebe59da6` |

## Marketplace refinement

The subsequent marketplace pass uses the requested `#ed860b` brand color with white/light-gray surfaces. [Shopee Vietnam](https://shopee.vn/) was referenced for shopping navigation, search prominence, catalog filters and order-state navigation; the Olist identity and original illustrations remain local.

Implemented on existing API layers: database-backed text search, currency/stock filters, safe sort options, listing detail, shop directory, buyer display-status filters, editable listing title/description/HTTPS image, selected-item checkout, and return-to-checkout after login. Seller review, KBS and phone verification flows remain in place. Listing content is an additive schema extension (migration `000007`); run migrations before starting the updated backend. Original Olist products and currency values are not rewritten.

Validation: PostgreSQL integration tests against both canonical and migrated schemas cover search escaping, price sorting/pagination, content edits and clearing, shop/buyer isolation, and URL validation. Browser checks at desktop/mobile sizes used a real local API and isolated database for login, purchase selection, checkout, seller edits and search. No production data was modified.

Scope limits: product images use seller-provided HTTPS URLs (no upload storage); cart persistence remains browser-local; non-COD payment methods retain the existing demo behavior (no payment gateway). Coupons, chat, reviews, promotion campaigns and shipping-provider integrations are not implemented in this pass.

## Seller portal refinement

The seller workspace now uses the same orange brand with a white sidebar/top bar, responsive navigation, dashboard KPI cards, order review tabs and filters, product tables, shop selection cards, and restyled order review details. Seller styles live in `src/seller.css`; the superseded sidebar/metric rules were removed from `index.css`. Counts and filters continue to use the existing shop-scoped APIs. Risk badges and the score bar retain severity colors, including green for low and amber for medium risk.

Verified at 1440px and 390px: all seller routes, drawer navigation, horizontally scrollable tables, review tabs, and a simulated stale-assessment conflict preserving the reason and reusing the refreshed assessment ID. Real local API checks on an isolated database cover login, shop selection, dashboard, review filtering, order detail, product editing and category selection inside the product dialog. The shared select menu now renders above the modal overlay. Production/KBS live scoring was not exercised in this UI pass; no backend rules or schema changed in the seller refinement.

## Brand name

The user-facing brand is now `afd.` across marketplace, seller navigation, login, footer, browser title and favicon. The orange `#ed860b` palette stays in place. Olist names in dataset attribution, batch IDs and the original design references are retained.

## Product presentation

Products without a title use their category name without an ID suffix. Category labels replace underscores with spaces and capitalize the first letter, including buyer views, seller tables and category options. API filter values and stored identifiers remain unchanged.

Five locally hosted sample photos in `public/images/products/` are assigned consistently by product ID across the catalog, product detail, cart and seller table. A valid seller-provided HTTPS photo takes precedence; a failed image falls back to the sample, then the existing icon. The samples are illustrative and may not represent the real item. Generated with the built-in imagegen tool; exact prompts are recorded in [PRODUCT_IMAGE_PROMPTS.txt](../docs/PRODUCT_IMAGE_PROMPTS.txt). No seed or database changes are required.

The seller shop menu now wraps long names and scrolls within the available viewport. Shop cards and the switcher omit internal IDs/batch codes. Migration `000010` assigns persistent `Cửa hàng demo N` names to imported placeholders; seller navigation refreshes cached names from the API. Review status uses `Chờ người bán duyệt`, risk status uses `Chưa chấm điểm`, and shop totals count `đơn đã chấm điểm`.

Seller order detail is bundled with the main application, matching other routes. This avoids fetching a deleted lazy chunk when navigating from a tab opened before a deployment. Existing tabs using the previous build need one reload.
