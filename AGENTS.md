# IPChronicle Product Repository Instructions

## Authority And Scope

- Product and architecture decisions live in the
  [workspace repository](https://github.com/ipchronicle/workspace/tree/main/docs).
  Read the product definition, system architecture, UI information architecture,
  and relevant ADRs before changing behavior.
- This repository contains the complete first-release product. Do not recreate
  the reserved `server`, `web`, `agent`, or `deploy` repositories as component
  boundaries.
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
- Run `make check` for formatting, generated drift, lint, type checking, unit
  tests, Go race tests, and production builds.
- Run `make compose-smoke` when changing the center, web embedding, container,
  health endpoint, or Compose configuration.
- Run `make browser-test` when changing user-visible workflows.
- Before finishing, inspect `git diff` for secrets, generated artifacts,
  unrelated edits, duplicate logic, and unmentioned behavior changes.
- After a feature is complete and its risk-proportionate checks pass, create a
  dedicated local commit for that feature. Do not accumulate multiple finished
  features in the working tree; keep cross-repository commits aligned by
  purpose while preserving each repository's independent history.

Generated web assets, dependencies, local databases, `.env` files, coverage,
and VM state are not committed. Generated OpenAPI bindings are committed and
checked for drift.
