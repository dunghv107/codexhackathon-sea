# KBS order risk scoring

A FastAPI KBS service for the `olist_vn_sim` batch, following [PLAN_KBS](docs/plan/PLAN_KBS.md).
The service lives entirely in `kbs/`; the platform's `backend/` and `frontend/` remain separate.
The platform owns checkout creation, catalog, identity collection and delivery/case ingestion.
KBS reads their persisted `sfd` records, stores assessments and applies policy Hold.
Seller Hold/Release is owned by the authenticated Go API.

## KBS integration

Configure `DATABASE_URL` in an ignored `.env.kbs.local` file or inject it through the environment,
then start with `ENV_FILE=.env.kbs.local uv run python -m app.main`.
The database must contain the supplied `sfd` schema and simulation batch (backend migration 5).
Startup installs the unchanged draft configuration only when the batch has no rule versions.
Existing versions are preserved. Startup does not create tables or import dataset records.
Without `DATABASE_URL`, template endpoints remain available and KBS endpoints return `503`.

Requests use `/api/v1` and require `X-Shop-ID`. This header is a trusted integration boundary,
not authentication: the platform gateway must authenticate the caller, authorize the shop and
replace client-supplied shop headers. Do not expose these routes directly to untrusted clients.
Orders outside the selected shop or simulation batch return `404`; cross-shop history is returned
only as aggregate features, without phone/device/address keys or other shops' orders.

| Method and path | Behavior |
| --- | --- |
| `GET /orders` | Shop orders; optional `risk_level`, `review_status`, `limit` (1–100), `offset` |
| `GET /orders/{id}` | Latest assessment, aggregate facts/reasons, order events and decision audit |
| `POST /orders/{id}/score` | Reuse the assessment for the active version, or calculate and save it |
| `POST /orders/{id}/score?force=true` | Append an immutable assessment and report `matches_previous` |
| `POST /orders/{id}/explain` | Deterministic Vietnamese explanation, at most three sentences |
| `GET /rules` | Active, validated configuration from `rule_versions.config` |
| `GET /rules/versions` | Version metadata for the simulation batch |

Scoring and policy Hold share a transaction. Concurrent scorers lock target orders before checking
the cache. High scores hold unfulfilled orders and append a policy decision when the gate changes.
Historical fulfilled/canceled orders retain their workflow state. Low scores never automatically
release a hold. Seller decisions use Go's `/api/seller/shops/{shopId}/orders/{id}/decision`,
with the latest `assessment_id`, authenticated seller identity and nonempty reason.
The old Python decision route is removed. Go rejects stale assessments and checks workflow state.
Reasons serialize as `{rule_id, contribution, description}`; stored `points`/`message` remain readable.

Go calls KBS after checkout commits. On scoring failure the purchase remains saved and pending;
the seller can retry through the score button. New demo records use `olist_vn_sim` and `live:<uuid>`;
existing `app`/`olist_raw` records are preserved and cannot be scored by KBS.

The OOP implementation uses composition:

- `RuleConfig` validates versioned rule parameters and dependency order.
- `RuleEngine` is pure: observed facts and configuration produce score, reasons and unknown reasons.
- `RiskRepository` runs parameterized `rules/sql/facts.sql` for a batch of order IDs and stores assessments.
- `ScoringService` coordinates order locking, cache reuse, evaluation and immutable persistence.
- `OrderRepository` owns scoped reads; `OrderService` provides explanations.
- `Database` owns the PostgreSQL pool; FastAPI lifespan opens/closes it and readiness checks it.

All scoring history stays within `olist_vn_sim`. Cutoff is the target checkout's `placed_at`.
Historical identities, events and resolutions must be known strictly before cutoff; the target
identity may be available exactly at checkout time. Latest resolution is ordered by `decided_at`,
then `resolution_id` for deterministic ties. Velocity counts checkouts, not shop orders. Null keys
never match. Unknown facts do not fire rules and are recorded in `feature_snapshot.unknown_reasons`.
Weights, windows and thresholds come from the active configuration, not an LLM.

`app/rules/rules.json` is an unchanged copy of the data team's `draft-vn-olist-0.2` configuration.
Scores are demo risk scores, not calibrated probabilities. R9 remains disabled. LLM explanation,
network investigation and rule proposal/approval workflows are not implemented in this core service.

Backfill runs in batches of 1,000 orders, grouped by shop, without replacing existing assessments:

```bash
ENV_FILE=.env.kbs.local uv run python -m app.services.backfill
```

Dataset do thành viên team chuẩn bị trước hackathon từ Olist (công khai) + lớp mô phỏng có khai báo;
script sinh dữ liệu không thuộc bài nộp. See the dataset's
[README](docs/data-structure/olist_cod/olist_cod/README.md) and
[transformations](docs/data-structure/olist_cod/olist_cod/transformations.json).

## KBS verification

```bash
bash scripts/verify.sh
uv run pytest tests/unit/test_rule_engine.py -m unit
uv run pytest tests/integration -m integration
uv run pytest tests/acceptance -m acceptance
```

Integration tests require `KBS_TEST_DATABASE_URL` pointing to a dedicated, empty test database
with the supplied `sfd` schema already installed. Fixtures insert only small authored records,
then roll back each test; do not point them at a shared dataset database.
Acceptance tests require `KBS_ACCEPTANCE_DATABASE_URL` pointing to an already-loaded `olist_cod`
database. They run read-only SQL and compare all eight scenarios against the unchanged
`evaluation/scenario_expectations.json` contract. Neither lane reads `seed.sql`, CSV exports,
`latent_outcomes.jsonl` or the data team's evaluator. Missing database settings skip the relevant
lane; a skipped acceptance lane is not evidence of 8/8 correctness or dataset-scale performance.

## Start locally

Use Python 3.12 and `uv` to create the locked development environment. The repository already includes a development environment file; review `.env.dev` before starting and keep real credentials out of tracked files.

```bash
uv sync --locked
uv run python -m app.main
```

The default server listens on `http://localhost:1114`. Open `/docs` or `/scalar` for the development API reference. Check `/health` for liveness, `/ready` for baseline readiness, or `/api/v1/` for application metadata. Stop the local server with Ctrl+C.

```bash
scripts/verify.sh
```

`scripts/verify.sh` runs Ruff formatting and lint checks, Ty type checking, and the default local test lane. CI runs the same gate after `uv sync --locked --all-groups` and also builds the container image.

## What is included

| Area | Current implementation |
| --- | --- |
| HTTP | FastAPI application factory, versioned router, OpenAPI and Scalar in development |
| MCP | Official SDK v2, opt-in Streamable HTTP at `/mcp/` with bearer/Host/Origin protection, independent stdio, shared health and metadata services |
| Operations | Root liveness and readiness endpoints, request ID middleware, CORS configuration |
| Configuration | Pydantic Settings loaded from `ENV_FILE` or `.env.dev`, with environment variable overrides |
| Logging | Console and rotating file handlers; JSON format can be enabled through settings |
| Delivery | Locked Python dependencies, Dockerfile, development/staging/production Compose files, CI quality and image build jobs |
| Quality | Ruff, Ty, pytest lanes, branch coverage report command, GitNexus guidance for code changes |

Readiness checks PostgreSQL when `DATABASE_URL` is configured; liveness remains independent of it.

## Tooling for contributors

| Tool | Role in this repository | Required for |
| --- | --- | --- |
| Python 3.12 and `uv` | Runtime and locked Python environment | Local app and Python checks |
| Ruff | Formatter and linter, installed by `uv sync --locked` | Python changes and CI |
| Ty | Static type checker, installed by `uv sync --locked` | Python changes and CI |
| pytest, HTTPX, pytest-cov | Local tests and coverage, installed with the dev group | Test and coverage commands |
| GitNexus CLI | Dependency/impact index; repository alias `fastapi_template` | Agents and contributors changing existing code or making commits under [repository rules](AGENTS.md) |
| Node.js | Runs GitNexus CLI and the local `.gitnexus/run.cjs` status helper; GitNexus 1.6.12 requires `^22.18.0` or `>=24.11.0` | GitNexus commands, even when installed through Bun |
| Docker with Compose | Container build and environment-specific local deployment | Container workflows |
| Draw.io (diagrams.net) | Edits `.drawio` sources and exports diagram screenshots | Creating or updating project diagrams; Desktop CLI is needed for command-line PNG export |
| Bun | Runs `bunx` for agent skill installation; can also manage JS/TS tooling | Running `scripts/install-skills.sh`, or optional JS/TS work |
| `jq` | Reads and validates the skill source catalog | Running `scripts/install-skills.sh` |

Ruff and Ty are declared in the `dev` group of [`pyproject.toml`](pyproject.toml); do not install separate global copies to satisfy this project's checks. GitNexus, Draw.io, and Bun are external tools and are not Python dependencies. On a fresh checkout, create the local GitNexus index with `scripts/gitnexus-refresh.sh`; later check `node .gitnexus/run.cjs status` and refresh through the same script when stale. See the [GitNexus guide](.agents/gitnexus-guidelines.md).

Installing GitNexus or its skills does not install Node.js. If `gitnexus --version` fails with `/usr/bin/env: node: No such file or directory`, install a supported Node.js runtime and ensure `node --version` works in the same shell before refreshing the index.

Optional coding-agent skills are listed in [`skills-catalog.json`](skills-catalog.json). With Bun and `jq` installed, run [`scripts/install-skills.sh`](scripts/install-skills.sh) to install each listed skill through `bunx skills add`. The entries identify upstream sources but do not pin revisions; review them before running the installer. The catalog remains unchanged; the CLI writes its own ignored `skills-lock.json` in the repository root. The installer attempts every entry, reports failed skill names, and exits with status 1 if any failed. An optional first argument selects another catalog file; its filename must differ from `skills-lock.json`.

The installer targets both Codex and Claude Code. Codex reads the downloaded skills in `.agents/skills/`; the CLI creates Claude Code links in `.claude/skills/`. Both directories are ignored by Git. [Codex](https://developers.openai.com/codex/skills/) can select skills from their descriptions or through an explicit `$skill-name` mention; [Claude Code](https://code.claude.com/docs/en/skills) can select them when relevant or through `/skill-name`. Discovery does not mean every skill runs on every request, and individual skill policies can disable automatic invocation. If a new skill does not appear, restart the agent session.

## Repository layout

```text
app/
  main.py       importable app and local server entry point
  api/          app factory, middleware, dependencies, versioned endpoints
  core/         settings, logging, request context
  mcp/          explicit MCP capabilities, HTTP authentication, stdio entry point
  schemas/      Pydantic API contracts
  services/     behavior independent of HTTP transport
  agents/       empty until an agent has a defined contract
  pipelines/    empty until a multi-step workflow is needed
  rules/        versioned configuration, pure OOP engine and SQL facts
  db/           PostgreSQL pool
  repositories/ scoped SQL reads and transactional persistence
  models/, common/, utils/   extension packages
tests/          in-process API and service checks, pytest lane contract
scripts/        verification, test, coverage, and GitNexus commands
.agents/        coding, testing, writing, and diagram rules
```

The request path is `app/main.py` → `app/api/app.py` → versioned endpoint → service → response schema. Start a new API capability by defining its input/output contract, adding the endpoint and reusable service behavior, and checking the public response. For AI features, define provider permissions, validation, failure behavior, and audit evidence before adding code to `app/agents/` or `app/pipelines/`. The [architecture guide](docs/fastapi-template-architecture.md) explains the existing boundaries.

## Start with MCP

Follow the [MCP quickstart](docs/mcp/quickstart.md) to create a private token file
and connect over HTTP. HTTP is disabled by default, so a fresh clone needs no
MCP credential for REST. The local stdio example runs immediately:

```bash
uv run python examples/mcp_client.py stdio
```

Both transports expose `get_health_status` and the JSON resource `app://info`.
The [MCP documentation](docs/mcp/README.md) covers SDK research, capability
extension, configuration and remote deployment. Coding agents also read the
[MCP guidelines](.agents/mcp-guidelines.md). The shared bearer token is for
team-controlled internal clients; external-client OAuth is a separate integration.

## Configuration and deployment

[`app/core/config.py`](app/core/config.py) defines defaults and supported environment variables. `ENV_FILE` selects an environment file and defaults to `.env.dev`. [`.env.example`](.env.example), [`.env.dev`](.env.dev), [`.env.staging`](.env.staging) and [`.env.production`](.env.production) are versioned templates with all supported setting names, defaults and explicit placeholders. The three Compose files select `.env.dev`, `.env.staging`, and `.env.production` respectively.

Production mode rejects `CORS_ALLOW_ORIGINS=*`. Set explicit trusted origins, review `DOCS_ENABLED`, and disable `SERVER_RELOAD` for deployments. The production Compose command runs Uvicorn without reload. Store secrets through the deployment environment; tracked example or environment files must not contain real credentials.

Agree actual origins, MCP Host values and credentials with DevOps before
deployment. The MCP token placeholder cannot authenticate enabled HTTP.
Use deployment secret injection or ignored `.env.*.local` files for real
credentials; select a private file with `ENV_FILE` for local runs. Empty
Origin/exposed-header lists are intentional defaults rather than missing values.

For local container development:

```bash
docker compose --env-file .env.dev -f docker-compose.dev.yml up --build
```

## Quality and test lanes

```bash
uv run ruff format --check
uv run ruff check
uv run ty check
scripts/test-suite.sh default
scripts/test-report.sh default
```

The default lane runs unit and component tests, including local MCP stdio subprocesses, without optional infrastructure. `integration`, `acceptance`, and `live` are separate lanes for projects that add their fixtures and dependencies; live provider tests require explicit authorization. `scripts/test-report.sh default` writes local coverage artifacts under `reports/tests/` without a coverage threshold. See the [testing guide](.agents/testing-guidelines.md) for lane rules.

Coding agents start with [AGENTS.md](AGENTS.md); Claude Code uses [CLAUDE.md](CLAUDE.md) as a pointer to the same rules. The [contributor guide](.agents/README.md) maps conventions to each kind of task. For diagrams, use the [component library](assets/diagrams/README.md) and follow the [diagram guide](.agents/diagram-guidelines.md) for the editable source, screenshot, reading guide, and references.
