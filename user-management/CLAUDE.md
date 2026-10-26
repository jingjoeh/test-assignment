# CLAUDE.md — User Management API

## Source of truth (in priority order)
1. `../TEST-ASSIGNMENT.md`, section "User Management API": the original brief. If anything conflicts with it, the brief wins.
2. `docs/IMPLEMENTATION_PLAN.md`: the plan. Decisions in Part A (D1–D19) are binding.
3. `docs/requirements.md`: summary of the brief.

Read the plan before starting any implementation work.

## Workflow
- Implement the plan phase by phase, starting at B0. Don't skip ahead to Part C before Part B is done; C5 gRPC is a stretch goal (D10).
- Tick a checkbox (`[ ]` → `[x]`) only when that task is actually done. A phase is done only when its **Verify** list passes.
- Part A has no checkboxes; it records decisions, not tasks.
- If a decision needs to change, update the plan's Decision log and Part A first, then the code. Don't silently deviate.

## Commands (run from `user-management/`)
```sh
go build ./...
go vet ./...
gofmt -l .
go test -race -cover ./...
```

## Hard rules
- Hexagonal layers (D1): `domain` imports nothing; `service` imports only `domain` and `port`; only `adapter/mongo` imports the Mongo driver; `cmd/api` wires everything.
- HTTP: stdlib `net/http` only (D4). No router or web framework.
- Tests: stdlib `testing` with hand-written fakes (D5). No mocking libraries, no `time.Sleep` in tests (D16).
- Users are created only by `AuthService.Register` (D14). There is no `POST /users` and no `UserService.Create`.
- Import our adapters with the aliases `httpadapter` and `mongoadapter` wherever `net/http` or the Mongo driver is also imported (D19).
- Don't add a dependency that isn't listed in Part A of the plan without asking first.
