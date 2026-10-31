# KBS Explain and Network

Both routes require the existing trusted `X-Shop-ID` boundary and the `olist_vn_sim` batch.
The Go gateway must authenticate and authorize callers before forwarding that header.
Neither route changes scores, rules, review status or seller decisions.

## Explain

`POST /api/v1/orders/{id}/explain` checks ownership and loads the latest stored assessment before
consulting the cache. It returns `assessment_id`, `source` (`template` or `llm`), one to three
Vietnamese `sentences`, and `fallback_reason` (null on provider success).

The optional model selects up to three distinct **already-fired rule IDs** using contributions
and finite numeric aggregate facts. No order, shop, buyer or identity IDs, case messages or raw
addresses are sent. Descriptions and points in the displayed sentences come from the stored
reasons, not generated text. `source=llm` means LLM-assisted evidence selection, not free-form
generation or a model-generated risk score. A zero-rule assessment uses a template that explicitly
says missing data is not evidence of safety.

The adapter uses OpenAI Responses with strict JSON Schema and `store=false`, following the
[Structured Outputs contract](https://developers.openai.com/api/docs/guides/structured-outputs).
Local validation rejects duplicate IDs, unfired rules, extra fields, invalid JSON and more than
three IDs. The provider has no tools or write authority. It cannot recommend or apply a review decision.

Provider use is off by default. Configure an authorized deployment privately:

```dotenv
KBS_EXPLAIN_ENABLED=true
KBS_EXPLAIN_MODEL=YOUR_STRUCTURED_OUTPUT_MODEL
KBS_LLM_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=YOUR_PRIVATE_KEY
```

Choose a model available to your account that supports Responses structured outputs. Enabling
without a model/key fails settings validation; merely having an API key does not enable calls.
The lifespan owns and closes the HTTP client. HTTP operations have a three-second timeout and
the whole selection has a four-second budget; there are no automatic retries or redirects.

Fallback reasons are `disabled`, `no_fired_rules`, `provider_timeout`, `provider_unavailable`,
`provider_refusal` and `invalid_output`. Provider failures return a usable template without
exposing provider payloads. Failures are not cached, so a later request can retry. Successful
results and deterministic templates use a 256-entry LRU cache per application process, keyed by
immutable assessment ID. A new assessment misses the old entry; restart clears the cache.
Concurrent cache misses may call the provider separately; the cache is not a distributed quota.

## Network

`GET /api/v1/orders/{id}/network` returns:

- `order_id` and `input_cutoff_at` (target checkout's `placed_at`).
- `nodes`: `{id, kind, label, count}`, where kind is `checkout`, `identity` or `aggregate`.
- `edges`: `{source, target}` and `truncated` indicating omitted own-shop historical checkouts.

The graph is exactly checkout → identity type → historical checkout/aggregate. Target identity
must be available at or before cutoff. Historical checkout and identity availability must both
be strictly before cutoff. NULL identity keys never match, and raw batches are excluded.

Only target identity types are shown, never their values. Own-shop historical checkout IDs are
replaced with response-local node IDs. Checkouts with no order in the requesting shop become
one count per matching identity type, labelled “Checkout shop khác”; no foreign order/shop IDs
or individual foreign checkout nodes are returned. Each count deduplicates checkouts across shops.
Counts for phone, device and address can overlap and must not be summed as unique customers/orders.

The output reserves space for the target, three identity nodes, three foreign aggregates and
193 own-shop checkout nodes, at most 200 total. SQL limits the own-shop rows and computes foreign
counts in PostgreSQL. The small fixed-depth graph uses typed node/edge lists without a graph-library
dependency. It is evidence of a shared identifier, not proof of fraud; absence of links may mean
unknown identity. This view spans available prior history and does not replace each scoring rule's
specific time window.

This change exposes the internal KBS route only. Gateway routing and frontend visualization are
separate integration work. No live provider or production/dataset-scale performance is claimed by
the mocked provider tests and isolated PostgreSQL fixtures.

For LiteLLM Proxy, set `KBS_LLM_BASE_URL` to its API base (including `/v1` if needed),
`KBS_EXPLAIN_MODEL` to the proxy alias and `OPENAI_API_KEY` to the proxy key.
The adapter appends `/responses`; the proxy/model must support Responses structured outputs.
There is no automatic fallback to Chat Completions.
