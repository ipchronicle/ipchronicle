# IPChronicle Product Repository Instructions

## Authority And Scope

- Product and architecture decisions live in the
  [workspace repository](https://github.com/ipchronicle/workspace/tree/main/docs).
  Read the product definition, system architecture, UI information architecture,
  and relevant ADRs before changing behavior.
- Online documentation for the latest stable release lives in
  [`ipchronicle/ipchronicle.github.io`](https://github.com/ipchronicle/ipchronicle.github.io).
  This repository retains version-frozen offline documentation that is shipped
  with release assets.
- This repository contains the complete product. A new component repository
  requires an accepted repository-boundary decision; do not split code merely
  because Center, Web, Agent, and deployment assets have different runtime roles.
- When these instructions conflict with an accepted ADR, the ADR is
  authoritative. Do not preserve a conflicting scaffold default.

## Repository Ownership

- `cmd/ipchronicle-center/`: center process entry point.
- `cmd/ipchronicle-agent/`: root Agent process entry point.
- `internal/center/`: center HTTP composition and application modules.
- `internal/center/database/`: embedded migrations, hand-written SQL, and the
  generated `sqlc` query packages for the separately owned databases.
- `internal/agent/`: Agent application modules.
- `internal/generated/`: generated Go API bindings; never edit manually.
- `internal/webui/`: compiled web asset embedding and SPA serving boundary.
- `openapi/`: normative OpenAPI 3.1 contract and generator configuration.
- `web/`: Vite, React, TypeScript, shadcn/ui, and translation source.
- `deploy/`: supported Docker Compose deployment assets.
- `scripts/`: reproducible repository checks and build orchestration.

Add a nested `AGENTS.md` only when a directory develops substantial local
rules that would make this file ambiguous. Do not create empty rule files.

## Engineering Rules

- Prefer explicit, readable implementations and shallow control flow. Add an
  abstraction only when it protects a real invariant or removes meaningful
  duplication.
- Keep changes within the owning module. Do not introduce compatibility with
  Komari, legacy APIs, legacy databases, or reserved repositories.
- Do not add a dependency without explaining why the standard library and
  existing dependencies are insufficient.
- Never hide failures behind empty data, mock success, silent fallback, or
  broad error swallowing.
- Do not leave a behavior-affecting decision as a TODO and describe the work
  as complete.

## Frontend

- Use React function components and TypeScript strict mode.
- Use official shadcn/ui source components, Tailwind utilities, and shared
  theme tokens before writing standalone CSS or custom controls.
- All IPChronicle-owned visible copy, validation feedback, tooltips, and
  accessibility labels must exist in both `zh-CN` and `en` resources.
- Use the configured `openapi-fetch` transport. Do not scatter raw `fetch`
  calls, API paths, authentication options, or duplicate transport types in
  components.
- Keep state in its smallest owner. A form, server cache, or global-state
  library requires demonstrated need before adoption.
- Cover loading, empty, offline, partial-success, failure, recovery, and
  destructive-confirmation states where the owning workflow can produce them.

## Documentation Language

- Public documentation for users, operators, and contributors defaults to
  Simplified Chinese. Put the English counterpart in a same-directory `.en.md`
  file and add reciprocal language links near the top of both versions.
- Keep both language versions aligned whenever their shared subject changes.
  Licenses, third-party notices, and other material that must retain prescribed
  legal text are exempt.

## Center And API

- HTTP handlers stay at the transport boundary. Domain invariants,
  persistence, and durable state transitions belong to their application
  modules.
- Chi remains at HTTP composition boundaries. Domain code accepts standard
  `context.Context` and application types.
- `openapi/openapi.yaml` is the HTTP contract source of truth. Never edit
  generated Go or TypeScript bindings.
- `internal/center/database/*db/db.go`, `models.go`, and `queries.sql.go` are
  generated from `sqlc.yaml`, migrations, and `queries.sql`; never edit them
  manually.
- API errors use stable codes and structured parameters. The frontend
  localizes them; handler prose is not a browser contract.
- Center SQL will use `database/sql`, `sqlc`, and explicit parameterized SQL.
  Migrations are the schema source of truth, and `config.db` and `history.db`
  retain separate ownership.

## Agent

- Keep the Agent build free of CGO and portable across the supported glibc and
  musl Linux distributions on AMD64 and ARM64.
- Do not weaken root execution, outbound-only networking, bounded queues,
  configuration revision convergence, or crash-consistency rules.
- Upload retry is data retransmission and must never execute a probe again.
- Process execution must retain explicit time, output, process-tree, and
  diagnostic-redaction boundaries.

## Generated Files And Validation

- Run `make generate` after changing OpenAPI or generator configuration.
- Run focused tests while developing, then run `make preflight` before pushing.
  Keep this local gate fast: it checks release-version consistency, generated
  file state, diff integrity, forbidden artifacts, and obvious committed
  secrets without installing dependencies or running builds and test suites.
- `make ci` is the fast GitHub Actions gate for dependency installation,
  regeneration, formatting, lint, types, ordinary unit tests, OpenAPI, Go vet,
  module tidiness, and the production Web build.
- `make check` adds native release builds, no-CGO Agent builds, and Go race
  tests. It runs in scheduled, manually requested, and full release validation;
  run the relevant part locally only when diagnosing a CI failure.
- Compose, desktop/mobile browser, dual-architecture image, distribution,
  resource-limit, reproducibility, and release failure tests run in GitHub
  Actions. Daily CI selects direct dependencies by changed path, while release
  candidates run the gates required by their version and risk classification.
  Use the local targets only for focused diagnosis.
- A push is not release-ready until ordinary CI for that exact revision passes.
- Before finishing, inspect `git diff` for secrets, generated artifacts,
  unrelated edits, duplicate logic, and unmentioned behavior changes.
- After a feature is complete and its risk-proportionate checks pass, create a
  dedicated local commit for that feature. Do not accumulate multiple finished
  features in the working tree; keep cross-repository commits aligned by
  purpose while preserving each repository's independent history.

Generated web assets, dependencies, local databases, `.env` files, coverage,
and VM state are not committed. Generated OpenAPI bindings are committed and
checked for drift.
