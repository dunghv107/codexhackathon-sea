# Olist design handoff

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
- No fabricated filter counts, buyer status facets, payment status in summary rows, shop order totals or product photos. Product placeholders are intentional.
- Only the available local Be Vietnam Pro fonts were imported. IDs and JSON use the system monospace fallback.
- Backend validation remains authoritative. No scoring, schema, seed or KBS changes.

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
