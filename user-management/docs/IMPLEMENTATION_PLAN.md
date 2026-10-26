# User Management API — Implementation Plan

> Source of truth: [`TEST-ASSIGNMENT.md`](../../TEST-ASSIGNMENT.md), section "User Management API" (the original brief). If anything here conflicts with the brief, the brief wins.
> Summary used for this plan: [`requirements.md`](./requirements.md)
> Scope: plan only, no code. Checkboxes appear only in Parts B, C and the final checklist, and mean implementation tasks: tick each `[ ]` → `[x]` as it is done. Part A has no checkboxes because it records decisions, not work.
> Each phase ends with a **Verify** checklist that has to pass before the phase counts as done.

---

## Part A — Assumptions

These are the answers to the open points in `requirements.md` §5. They are **decided assumptions**, not implementation tasks: they have no checkboxes, and nothing in Part A is "done" until the matching tasks in Parts B and C are ticked. Each one also goes into the README's "Assumptions & Design Decisions" section (B9).

> **Status:** All assumptions in Part A are decided. Labels:
> - **✅ Decided (Dn)**: discussed and decided one by one (D1–D17, D19–D23).
> - **✅ Accepted default (D18)**: proposed in the first draft and accepted as a group without individual discussion.
>
> **Recommended ADRs (not yet written):**
> - ADR-001: Adopt hexagonal architecture (D1), with sub-sections on choosing stdlib `net/http` over a framework (D4) and on services not calling each other, with `AuthService.Register` owning user creation (D14)

### Decision log

| # | Topic | Decision | ADR? |
| --- | --- | --- | --- |
| D1 | Architecture | Hexagonal from the start (`domain` / `port` / `service` / `adapter` / `cmd`) | Recommended |
| D2 | Register vs create user | Only `POST /auth/register` (public). No `POST /users`. Registration covers "Create a new user" | No (README note) |
| D3 | Authorization | Any authenticated user can read, update and delete any user. No ownership check, no roles | No (README note) |
| D4 | HTTP router | Standard library `net/http` (`ServeMux` method and path patterns). No third-party router or framework | No (mention in ADR-001) |
| D5 | Mocking strategy | Hand-written in-memory fakes that implement the port interfaces. No mock generator, no mocking library. No MongoDB integration tests in the initial submission (reaffirmed 2026-10-26) | No (README note) |
| D6 | JWT details | Access token only, default TTL 1h (env), claims `sub`, `iss`, `iat`, `exp` (no email), `iss` validated. No refresh token | No (README JWT guide) |
| D7 | Update semantics | `PATCH /users/{id}`, partial update of `name` and/or `email`, at least one field. Password can't be changed | No (README note) |
| D8 | List users | `GET /users` returns every user as a plain JSON array. No pagination | No (README note) |
| D9 | Delete semantics | Hard delete. `204` on success, `404` when the ID doesn't exist | No |
| D10 | Bonus scope | Do C1, C3, C4 (C2 and C6 come with D1/D5). C5 gRPC is a stretch goal, only after Part B and the final checklist pass | No |
| D11 | Validation rules | Length and format rules as in C3 (no password composition rules). Email at most 254 bytes after normalization, on register and update. Unknown JSON fields are rejected with 400 | No (README note) |
| D12 | Go module | `go.mod` in `user-management/`, module path `github.com/jingjoeh/test-assignment/user-management` | No |
| D13 | MongoDB driver | Official driver v2 (`go.mongodb.org/mongo-driver/v2`) | No |
| D14 | User creation ownership | `AuthService.Register` is the only place that creates users. There is no `UserService.Create`, and services don't call each other | No (mention in ADR-001) |
| D15 | ID handling | Malformed ObjectID → 400 (`ErrInvalidID`). Well-formed ID that doesn't exist → 404 (`ErrNotFound`) | No (README note) |
| D16 | Worker testability | The worker reads ticks from an injected channel, so tests drive it by hand with no sleeps and no real ticker | No |
| D17 | Security hardening | Required JWT claims, `JWT_SECRET` of at least 32 bytes, HTTP server timeouts, 1 MiB request-body limit, trailing JSON rejected | No (README note) |
| D18 | Remaining defaults | Every first-draft default in Part A that wasn't discussed individually is accepted as written | No |
| D19 | Adapter package names | Keep `adapter/http` (package `http`) and `adapter/mongo` (package `mongo`). Importers that also need `net/http` or the Mongo driver use aliases `httpadapter` and `mongoadapter` | No |
| D20 | JSON field naming | camelCase JSON keys (`id`, `name`, `email`, `createdAt`). The domain entity has no JSON tags; the HTTP adapter maps it to a DTO | No (README samples) |
| D21 | Password maximum | Passwords are at most 72 bytes (bcrypt's input limit); longer ones are a 400 validation error, not a 500 | No (README note) |
| D22 | Go version | Minimum Go is 1.26 (`go 1.26.0`), the oldest release that still gets security fixes; dependencies on their latest versions; Docker builds with `golang:1.27`. Supersedes the D18 default of Go 1.23 | No (README note) |
| D23 | gRPC (C5) | `google.golang.org/grpc` v1.84.0 and `google.golang.org/protobuf` v1.36.12. Generated code committed; tools (protoc, plugins, grpcurl) only in the session scratchpad. `CreateUser` public, `GetUser` needs a bearer token (mirrors REST). Server reflection on | No (README note) |

### A0. Architecture
- Use hexagonal architecture (ports and adapters) from the first commit, not as a later refactor. ✅ Decided (D1)
- Layers: `domain` (entities and errors, no dependencies) → `port` (interfaces) → `service` (application logic) → `adapter/http`, `adapter/mongo` (and later `adapter/grpc`) → `cmd/api` (wiring). ✅ Decided (D1)
- Keep it to these core layers only, with no extra sub-layers, to avoid over-engineering. ✅ Decided (D1)
- Explain the reasoning in the README's architecture section (C6). ✅ Decided (D1)
- Adapter folders keep the names `adapter/http` and `adapter/mongo`, so their packages are `http` and `mongo`. A file that imports one of them together with `net/http` or the driver's `mongo` package aliases ours: `httpadapter ".../internal/adapter/http"` and `mongoadapter ".../internal/adapter/mongo"`. Use these two aliases everywhere for consistency. ✅ Decided (D19)

### A1. Tech stack
- Go 1.26+ (`go 1.26.0` in `go.mod`). Go supports only its two newest releases (1.27 and 1.26 as of 2026-10); 1.23, the first-draft default, no longer gets security fixes and held the dependencies back. Since Go 1.21, an older local `go` downloads the required toolchain automatically, so a higher minimum rarely blocks anyone. The Docker image (C1) builds with `golang:1.27`. ✅ Decided (D22, supersedes the D18 default)
- HTTP router: the standard library `net/http` (`ServeMux` with method and path patterns), so there is no framework dependency. Middleware is `func(http.Handler) http.Handler`, and small JSON helpers are written in `adapter/http`. ✅ Decided (D4)
- MongoDB 7.x server. ✅ Accepted default (D18)
- Official driver v2: `go.mongodb.org/mongo-driver/v2` (v2.9.2). Only `adapter/mongo` imports it (D1). ✅ Decided (D13)
- JWT: `github.com/golang-jwt/jwt/v5` (v5.3.1). ✅ Accepted default (D18)
- Password hashing: `golang.org/x/crypto/bcrypt` (x/crypto v0.58.0). ✅ Accepted default (D18)
- Dependencies stay on their latest versions, as long as a module's own `go` directive doesn't exceed 1.26 (D22); check it before upgrading
- Logging: the standard library `log/slog`, with JSON output to stdout. ✅ Accepted default (D18)
- Configuration through environment variables: `PORT`, `MONGO_URI`, `MONGO_DB`, `JWT_SECRET` (at least 32 bytes, D17), `JWT_TTL` (default `1h`, D6), `JWT_ISSUER` (D6). ✅ Accepted default (D18)

### A2. User model
- `ID` is a MongoDB `ObjectID`, exposed in the API as a hex string. ✅ Accepted default (D18)
- `Email` is trimmed, lowercased and stored that way; uniqueness comes from a unique index on `email`. ✅ Accepted default (D18)
- `Password` is a bcrypt hash at the default cost. It is never returned in any response. ✅ Accepted default (D18)
- `CreatedAt` is stored in UTC and returned as RFC 3339. ✅ Accepted default (D18)
- JSON keys are camelCase: a user is returned as `{ "id", "name", "email", "createdAt" }`. `domain.User` has no JSON or BSON tags; `adapter/http` maps it to a response DTO that has no password field. ✅ Decided (D20)

### A3. Authentication and authorization
- `POST /auth/register` is public and creates a user (name, email, password). ✅ Decided (D2)
- `AuthService.Register` is the sole owner of user creation: it validates input, normalizes the email, hashes the password, builds the `domain.User` (including `CreatedAt`) and calls `UserRepository.Create`. There is no `UserService.Create`, and no service calls another service. ✅ Decided (D14)
- `POST /auth/login` is public. It takes email and password and returns `{ "token": "..." }`. ✅ Accepted default (D18)
- There is no separate `POST /users`. Registration is the only way to create a user and covers the brief's "Create a new user" (req. 3), because a protected create-user endpoint has no clear use case. ✅ Decided (D2)
- Known risk: a grader checking req. 3 item by item may look for a create endpoint under `/users`. Mitigation: the README maps "Create a new user" → `POST /auth/register` explicitly (B9). ✅ Decided (D2)
- Every `/users` endpoint requires `Authorization: Bearer <token>`. ✅ Accepted default (D18)
- JWT claims are `sub` (user ID), `iss`, `iat` and `exp`. There is no `email` claim, so a token can't carry a stale email after an update. ✅ Decided (D6)
- The default TTL is 1h, configurable through `JWT_TTL`. ✅ Decided (D6)
- `iss` is set from `JWT_ISSUER` and checked on validation. A token with the wrong issuer is rejected. ✅ Decided (D6)
- All four claims are required on validation. A token missing `sub`, `iss`, `iat` or `exp`, or with an empty `sub`, is rejected with 401 even if its signature is valid. ✅ Decided (D17)
- There is no refresh token and no revocation. The README lists both under "Known limitations". ✅ Decided (D6)
- Signing uses HS256 only. Tokens signed with any other `alg` are rejected. ✅ Accepted default (D18)
- The service refuses to start if `JWT_SECRET` is shorter than 32 bytes (256 bits, the HS256 key size), which also covers an empty or missing secret. `.env.example` shows a placeholder and how to generate a real one (e.g. `openssl rand -base64 32`). ✅ Decided (D17)
- Authorization: any authenticated user can manage any user. There is no ownership check and there are no roles; this matches the brief, which only asks to protect endpoints with JWT. ✅ Decided (D3)
- Known limitation: one user can update or delete another user. The README lists this under "Known limitations" and names ownership checks or roles as possible future work. ✅ Decided (D3)

### A4. REST API contract

| Method | Path | Auth | Success |
| --- | --- | --- | --- |
| POST | `/auth/register` | public | 201 + user |
| POST | `/auth/login` | public | 200 + token |
| GET | `/users` | JWT | 200 + list |
| GET | `/users/{id}` | JWT | 200 + user |
| PATCH | `/users/{id}` | JWT | 200 + user |
| DELETE | `/users/{id}` | JWT | 204 |

- PATCH is a partial update: `name`, `email` or both. At least one field is required; an empty body or `{}` returns 400. ✅ Decided (D7)
- The request DTO uses pointer fields (`*string`) to tell "field not sent" apart from "field sent as empty". ✅ Decided (D7)
- The password cannot be changed through this endpoint or any other. ✅ Decided (D7)
- Unknown fields in the body (e.g. `password` on PATCH) are rejected with 400. ✅ Decided (D11)
- `GET /users` returns every user as a plain JSON array (`[...]`, `[]` when empty) with no pagination, as the brief says ("list all"). ✅ Decided (D8)
- Pagination is listed in the README under "Known limitations" as future work. ✅ Decided (D8)
- Delete is a hard delete: the document is removed from the collection, and there is no `DeletedAt` field. ✅ Decided (D9)
- `DELETE /users/{id}` returns `204` on success and `404` when the ID doesn't exist, including a second delete of the same ID. ✅ Decided (D9)
- For every `/users/{id}` route (GET, PATCH, DELETE), a malformed ObjectID returns `400`, and a well-formed ID with no matching user returns `404`. ✅ Decided (D15)
- Request bodies are limited to 1 MiB; a larger body returns `413`. ✅ Decided (D17)
- A body with extra data after the first JSON value (e.g. `{"name":"a"}{"name":"b"}`) returns `400`. ✅ Decided (D17)
- Error format is `{ "error": "<message>" }`. Status codes: 400 validation, malformed JSON or malformed ID; 401 auth; 404 not found; 409 duplicate email; 413 body too large; 500 internal (generic message, details only in the log). ✅ Accepted default (D18)

### A5. Middleware and background task
- The logging middleware logs method, path, status code and duration. Status code goes beyond what the brief asks for. ✅ Accepted default (D18)
- The user-count goroutine runs every 10s, so the first log appears 10s after start. Errors are logged and it keeps running. It stops when the root context is cancelled. ✅ Accepted default (D18)
- The worker doesn't create its own ticker. It receives ticks from an injected `<-chan time.Time`: `main` passes `ticker.C`, and tests pass a channel they control. ✅ Decided (D16)

### A5a. HTTP server hardening
- `http.Server` sets `ReadHeaderTimeout` 5s, `ReadTimeout` 10s, `WriteTimeout` 10s and `IdleTimeout` 60s. No server runs with the zero-value (unlimited) timeouts. ✅ Decided (D17)
- Values are constants in `cmd/api`, not env vars, since the brief doesn't need them configurable. ✅ Decided (D17)

### A6. Testing
- Unit tests use only the standard library `testing` package, with hand-written fakes and no mocking library or code generation. ✅ Decided (D5)
- Mocking happens at the repository interface, so service and handler tests don't need a real MongoDB. ✅ Decided (D5)
- The fake repository behaves like the real one where it matters (duplicate email → `ErrEmailExists`, missing ID → `ErrNotFound`) and has a settable error field so tests can force failures. ✅ Decided (D5)
- Integration tests against a real MongoDB are out of scope for the initial submission: no build tags, testcontainers, integration-test code or new dependencies. ✅ Decided (D5, reaffirmed 2026-10-26)
  - Why: the brief asks for unit tests with appropriate mocking; service and HTTP behavior is covered through the repository fakes; the Mongo adapter and the unique index were verified manually against `mongo:7` in B2, B8 and after the D22 driver upgrade; and B9, C4 and C1 have higher priority than another test mode
  - Consequence: `adapter/mongo` unit coverage stays low (about 11%: ID parsing, error mapping, document conversion). The README states this boundary so the number isn't read as an oversight
- Coverage target is at least 70% for the domain, service and handler packages. ✅ Accepted default (D18)
- Because mandatory requirement 7 needs MongoDB mocked, the interface abstraction (Bonus "Abstraction") gets built as part of the mandatory work. ✅ Decided (D1, D5, D10)

### A7. Repository layout
- All code and documentation for this part live under `user-management/`. That folder has its own `README.md` and `go.mod`. ✅ Decided (D12)
- The module path is based on the GitHub repo rather than a short local name. ✅ Decided (D12)
- Module path: `github.com/jingjoeh/test-assignment/user-management`. The GitHub repo was renamed from the misspelled `test-assingment` before any code was written. ✅ Decided (D12)

---

## Part B — Mandatory Requirements

### B0. Project skeleton
- [x] `go mod init github.com/jingjoeh/test-assignment/user-management` inside `user-management/` (D12), with the `go` directive pinned to the A1 minimum (now `1.26.0`, D22)
- [x] Create the folder structure (the hexagonal layout is also bonus C6):
  ```
  user-management/
  ├── cmd/api/                 # main: wiring, config, server start
  ├── internal/
  │   ├── domain/              # User entity, domain errors
  │   ├── port/                # interfaces: UserRepository, PasswordHasher, TokenIssuer, TokenValidator
  │   ├── service/             # AuthService (register, login), UserService (get, list, update, delete)
  │   ├── adapter/
  │   │   ├── auth/            # bcrypt hasher, HS256 JWT issuer/validator
  │   │   ├── http/            # handlers, middleware, DTOs
  │   │   └── mongo/           # repository implementation
  │   ├── fake/                # in-memory fakes of the ports for tests (D5)
  │   └── worker/              # background user counter
  └── docs/
  ```
- [x] Config loader that reads the env vars from A1 and fails fast when a required value is missing or `JWT_SECRET` is shorter than 32 bytes (D17)
  - Required: `MONGO_URI`, `MONGO_DB`, `JWT_SECRET`, `JWT_ISSUER`. Defaults: `PORT=8080`, `JWT_TTL=1h` (must be a positive duration)
  - All problems are reported together in one error

**Verify**
- [x] Config tests: missing required var fails, 31-byte secret fails, 32-byte secret passes, `JWT_TTL` defaults to `1h` and rejects an invalid duration
- [x] `go build ./...` succeeds
- [x] `go vet ./...` is clean

### B1. User model (req. 1)
- [x] `domain.User` with `ID`, `Name`, `Email`, `PasswordHash`, `CreatedAt`
- [x] Domain errors: `ErrNotFound`, `ErrInvalidID` (D15), `ErrEmailExists`, `ErrInvalidCredentials`, `ErrValidation` (wrapped with the specific problem via `fmt.Errorf("%w: ...", ErrValidation)`)
- [x] Response DTO `userResponse` in `adapter/http` (D20)

**Verify**
- [x] All five fields from the brief are present
- [x] The JSON response DTO has no password field (checked by `TestUserResponseJSON`: exact key set, no `password` and no hash in the output, `createdAt` in UTC)

### B2. MongoDB integration (req. 4)
- [x] `port.UserRepository` interface with `Create`, `GetByID`, `GetByEmail`, `List`, `Update`, `Delete`, `Count`. `Update` takes `port.UserUpdate{Name, Email *string}` (D7)
- [x] Mongo adapter that implements the interface with the official driver (v2.9.2 since D22; re-verified against `mongo:7` after the upgrade). `Update` uses `FindOneAndUpdate` so the change and the returned user are atomic
- [x] Create a unique index on `email` at startup (`EnsureIndexes`, idempotent; called from `main` in B8)
- [x] Map duplicate-key errors to `ErrEmailExists` and `ErrNoDocuments` to `ErrNotFound`
- [x] Map an invalid ObjectID hex string to `ErrInvalidID` (→ 400), and a valid ID with no document to `ErrNotFound` (→ 404) (D15). Parsing stays in `adapter/mongo`, so the domain keeps IDs as plain strings
- [x] Use a context with a timeout on every DB call
- [x] Connect at startup with a ping check (`Connect`)
- [x] Disconnect on shutdown. Deferred in `run` since B8, but `run` only returns cleanly once C4 handles signals, so it is checked in C4. Checked in C4: `mongo disconnected` is logged after `worker stopped` on SIGINT

**Verify**
- [x] When run against a local MongoDB, a created user can be read back
- [x] Inserting a duplicate email returns `ErrEmailExists`
- [x] `GetByID("not-an-id")` returns `ErrInvalidID`, and `GetByID` with a valid but unused ObjectID returns `ErrNotFound`
- Verified 2026-10-26 against `mongo:7` in Docker with a temporary build-tagged test (deleted afterwards, D5)

### B3. Authentication and JWT (req. 2)
- [x] Password hasher (bcrypt) behind an interface (`port.PasswordHasher`, `adapter/auth.BcryptHasher`)
- [x] Token issuer and validator (`port.TokenIssuer` / `port.TokenValidator`, `adapter/auth.JWT`; HS256 only, secret from config, `exp` and `iss` enforced, D6; `sub`, `iss`, `iat`, `exp` all required and `sub` non-empty, D17)
- [x] `AuthService.Register` (sole owner of user creation, D14): validate input, normalize the email, hash the password, set `CreatedAt`, call `UserRepository.Create` directly. It doesn't call `UserService`
- [x] `AuthService.Login`: look up by email, compare the hash, issue a token. Use the same error for an unknown email and a wrong password. An unknown email is still compared against a dummy bcrypt hash, so response time doesn't reveal which emails exist
- [x] `POST /auth/register` and `POST /auth/login` handlers and the router (`adapter/http`)
- [x] JWT middleware: parse the `Bearer` header, validate the token, put the user ID in the request context, and return 401 on any failure

**Verify**
- [x] Login with valid credentials returns a token that decodes with `alg: HS256`
- [x] Without a token, a protected route returns 401 (checked in B4 by `TestUsersRoutesRequireToken` on every `/users` route)
- [x] An expired, tampered, wrong-alg or wrong-issuer token returns 401
- [x] A correctly signed token missing `sub`, `iss`, `iat` or `exp`, or with an empty `sub`, returns 401 (D17)
- [x] A wrong password and an unknown email return the same 401 response

### B4. User operations (req. 3)
- [x] `UserService`: `Get`, `List`, `Update` (name and/or email), `Delete`. No `Create`: creation belongs to `AuthService.Register` (D14)
- [x] Handlers and routes for `/users` as in A4, all wrapped in the JWT middleware
- [x] Map domain errors to HTTP status codes in one place (`ErrInvalidID` → 400, `ErrNotFound` → 404, D15)
- [x] One shared JSON decode helper used by every handler that reads a body:
  - [x] Wraps the body in `http.MaxBytesReader` (1 MiB); exceeding it returns 413 (D17)
  - [x] Uses `DisallowUnknownFields`, so unknown fields return 400 (D11)
  - [x] After decoding one value, checks that the rest of the body is empty (a second `Decode` must return `io.EOF`); otherwise 400 (D17)
  - [x] Malformed or empty JSON returns 400
- [x] One shared JSON response helper that sets `Content-Type: application/json` before writing the status, used for every JSON success and error response
- [x] `DELETE` success writes `204` with no body and doesn't go through the JSON response helper
- [x] Unknown routes return a JSON 404 (`{"error": ...}`, `Content-Type: application/json`) instead of the `ServeMux` plain-text body. `jsonMuxErrors` wraps the mux; responses it doesn't recognize (such as path-cleaning redirects) pass through unchanged
- [x] An unsupported method on a known route returns a JSON 405 and keeps the correct `Allow` header (e.g. `GET /auth/login` → 405, `Allow: POST`)
- [x] `PATCH` validates `email` with the same `validateEmail` as register, including the 254-byte limit (D11)

**Verify**
- [x] Each of the five operations works end-to-end with curl (create = `POST /auth/register`). Verified 2026-10-26 against the running binary and `mongo:7` in Docker
- [x] Updating to an email that already exists returns 409
- [x] PATCH with only `name`, only `email`, or both updates just those fields; PATCH with `{}` returns 400 (D7)
- [x] Get, update or delete on a well-formed but missing ID returns 404, and on a malformed ID returns 400 (D15)
- [x] A body over 1 MiB returns 413, and a body with trailing JSON returns 400 (D17)
- [x] Tests: an unknown route returns a JSON 404; `GET /auth/login`, `PUT /users/{id}` and `POST /users/{id}` return a JSON 405 with the right `Allow` header
- [x] No response contains a password or hash
- [x] End-to-end against the running application and MongoDB: register a user, delete it, then register again with the same email; the second registration returns 201 (D9: hard delete frees the unique email). Verified 2026-10-26 against the running binary and `mongo:7` in Docker

### B5. Logging middleware (req. 5)
- [x] Middleware that records start time, wraps `ResponseWriter` to capture the status, and logs method, path, status and duration (`Logging`; fields `method`, `path`, `status`, `duration_ms`; the path excludes the query string)
- [x] Applied to every route, public ones included: it wraps the whole router, outside `RequireAuth` and the JSON 404/405 wrapper

**Verify**
- [x] Every request, including public routes, error responses and requests rejected by the JWT middleware, produces exactly one log entry containing method, path, status code and duration (`TestRouterLogsEveryRequestOnce`: 200, 201, 204, 400, 401, 404, 405 and 500). A 500 additionally logs the error itself in a separate `request failed` entry

### B6. Concurrency task (req. 6)
- [x] Worker that takes a context, a tick channel (`<-chan time.Time`), a `Count` dependency (an interface) and a `*slog.Logger` (D16): `worker.RunUserCount`, with `worker.UserCounter` (satisfied by `port.UserRepository`)
- [x] On each tick it calls `Count` and logs `total_users=<n>` (INFO `user count`); on error it logs the error (ERROR `user count failed`) and waits for the next tick
- [x] `Run` returns when the context is cancelled or the tick channel is closed
- [x] `main` creates the 10s `time.Ticker`, passes `ticker.C`, runs the worker in its own goroutine and stops the ticker on shutdown (deferred in `run`; exercised on a real shutdown in C4)

**Verify**
- [x] Manual smoke check: after the app runs for about 30s, the log has about 3 user-count lines. Verified 2026-10-26 against the running binary and `mongo:7` in Docker: ticks exactly 10s apart
- [x] Manual smoke check: registering a user changes the count in the next tick (0 → 2 after the curl run registered users). Verified 2026-10-26 against the running binary and `mongo:7` in Docker
- [x] The goroutine exits when the context is cancelled (checked by a deterministic test, B7: `TestRunUserCountStopsOnCancel`; all worker tests pass 200 runs in a row under `-race`)

### B7. Testing (req. 7)
- [x] Fake `UserRepository` (in `internal/fake`; an in-memory map with a settable error for failure cases, D5) for service and handler tests
- [x] Fakes for `PasswordHasher` and `TokenIssuer`/`TokenValidator` where a test needs to isolate the service from bcrypt or JWT
- [x] `AuthService` tests:
  - [x] Register: success, duplicate email → `ErrEmailExists`, duplicate email differing only in case or surrounding spaces → `ErrEmailExists`, invalid input → `ErrValidation` with no repository call
  - [x] Register: the stored user has a bcrypt hash (not the plain password), a lowercased email and a non-zero UTC `CreatedAt`
  - [x] Register: a hasher error or repository error is returned and nothing is half-created
  - [x] Login: success returns a token whose `sub` is the user ID; email lookup is case-insensitive
  - [x] Login: wrong password and unknown email both return `ErrInvalidCredentials`; a repository error is not turned into `ErrInvalidCredentials`
- [x] `UserService` tests:
  - [x] Get: success, `ErrNotFound`, `ErrInvalidID`
  - [x] List: empty repository returns an empty (non-nil) slice; several users are all returned
  - [x] Update: name only, email only, both, no fields → `ErrValidation`, duplicate email → `ErrEmailExists`, updating to the user's own current email succeeds, email normalized, `ErrNotFound`, `ErrInvalidID`
  - [x] Update leaves `CreatedAt` and `PasswordHash` unchanged
  - [x] Delete removes the user document: a later Get of the same ID returns `ErrNotFound` (D9)
  - [x] Delete: success, `ErrNotFound`, `ErrInvalidID`
  - [x] Every method passes repository errors through
- [x] Bcrypt tests with the real hasher: a 72-byte password plus any suffix doesn't match (regression), plus an HTTP login test wired with the real hasher
- [x] Token tests: issue then validate, expired token, tampered token, wrong alg (including `none`), wrong issuer, wrong secret, and each required claim missing (D17)
- [x] Middleware tests:
  - [x] JWT: missing header, wrong scheme, missing token, extra fields, invalid/expired/wrong-issuer token → 401; `bearer`/`BEARER`/mixed-case scheme and extra or surrounding whitespace are accepted; a valid token puts the user ID in the request context
  - [x] Logging: method, path, status and duration are logged, including when the handler never calls `WriteHeader` (status 200)
- [x] Handler tests with `httptest`, for every route:
  - [x] Success status and JSON body; no `password` or hash field in any response
  - [x] Every response that has a JSON body, success or error, sets `Content-Type: application/json`
  - [x] `DELETE` success returns 204 with an empty body (no JSON, so no JSON `Content-Type` is required)
  - [x] `POST /auth/register`: 201, 400 (validation, unknown field, malformed JSON, trailing JSON), 409, 413
  - [x] `POST /auth/login`: 200 with token, 400 (missing field, malformed JSON), 401 with the same body for wrong password and unknown email
  - [x] `GET /users`: 200 with `[]` when empty, 200 with users
  - [x] `GET /users/{id}`: 200, 400 malformed ID, 404 missing ID (D15)
  - [x] `PATCH /users/{id}`: 200, 400 (`{}`, unknown field such as `password`, invalid email, trailing JSON), 400 malformed ID, 404, 409, 413
  - [x] `DELETE /users/{id}`: 204 with empty body, 400 malformed ID, 404 missing ID, 404 on second delete
  - [x] Every `/users` route returns 401 without a valid token
  - [x] A repository failure returns 500 with a generic message that doesn't leak the internal error
- [x] Worker tests, deterministic with no `time.Sleep` and no real ticker (D16):
  - [x] The test owns an unbuffered tick channel and runs `Run` in a goroutine with a `done` channel
  - [x] Each unbuffered send blocks until the worker has taken the tick, so the test controls exactly how many ticks happen
  - [x] After the last tick, the test cancels the context and waits for `done`; because `Run` has returned, all log writes are finished and the captured log (a `bytes.Buffer` behind the `slog` handler) can be asserted
  - [x] Cases: N ticks → N `total_users` lines with the fake counter's values; counter error → error logged and the next tick still counts; cancel → `Run` returns; closed tick channel → `Run` returns
  - [x] Waiting on `done` uses a generous `select` timeout only as a failsafe to fail the test, never to drive it

**Verify**
- [x] `go test ./...` passes
- [x] `go test -race ./...` passes
- [x] `go test -cover ./...` meets the A6 target (70% for domain, service and handler packages): `service` 100%, `adapter/http` 97.1%; `domain` has no statements

### B8. Wiring (`cmd/api`)
- [x] Load config, then connect Mongo, then build adapters, services, handlers and router, then start the worker, then start the HTTP server (`run(ctx, logger, getenv)`; cleanup is deferred so C4 only has to add signal handling)
- [x] Build `http.Server` with the timeouts from A5a (D17), never `http.ListenAndServe` with the default server (`newServer`, checked by `TestNewServerSetsTimeouts`)

**Verify**
- [x] `go run ./cmd/api` starts and serves requests against a local MongoDB. Verified 2026-10-26 against the running binary and `mongo:7` in Docker
- [x] `go run ./cmd/api` with a `JWT_SECRET` under 32 bytes exits with a clear error before connecting to Mongo (`TestRunRejectsShortSecretBeforeConnecting`, plus the B0 manual run)

### B9. Deliverables (documentation)
- [x] `user-management/README.md`
  - [x] Prerequisites and environment variables
  - [x] Setup and run instructions (local and Docker if C1 is done)
  - [x] How to run the tests
  - [x] Testing boundary: what the unit tests cover (domain, services, HTTP handlers and middleware, JWT, bcrypt, worker, config) through hand-written fakes; that MongoDB is deliberately not exercised by `go test` (D5); and why `adapter/mongo` coverage is low
  - [x] Manual MongoDB verification record: what was checked against `mongo:7` in Docker and when (B2 adapter checks, B8 end-to-end run of every endpoint, re-check after the driver v2.9.2 upgrade), including the unique email index, duplicate email, malformed vs missing ID, partial update, delete and re-register of a deleted email
- [x] JWT guide: how to register or log in to get a token, how to send it in the `Authorization` header, the required claims, and how to generate a `JWT_SECRET` of at least 32 bytes (D17)
- [x] Sample requests and responses (curl) for every endpoint, including error examples
- [x] Requirement-to-endpoint table that maps each brief item to its endpoint, including "Create a new user" → `POST /auth/register` (D2)
- [x] Assumptions & Design Decisions section, copied from Part A and kept up to date
- [x] Known limitations section, including the authorization limitation from D3, no refresh token or revocation from D6, and no pagination from D8
  - [x] State explicitly: a token belonging to a deleted user stays valid until it expires, because tokens are stateless and there is no revocation (D6)

**Verify**
- [x] Following the README from a fresh clone, the app runs and the sample requests work as documented. Checked 2026-10-26 on a clone of `8e326ed` plus the new README: the documented test commands, the `export` lines and `go run ./cmd/api`, the Try-it steps and the samples all matched (port 8089 because 8080 is taken on this machine). The final checklist repeats this on a real clone after the README is committed

---

## Part C — Bonus (Optional)

Scope (D10):

| Bonus | Status |
| --- | --- |
| C1 Containerization | In scope |
| C2 Abstraction | In scope, built as part of Part B (D1, D5) |
| C3 Validation | In scope |
| C4 Graceful shutdown | In scope |
| C5 gRPC | Stretch goal: start only after Part B, C1–C4, C6 and the final checklist are done |
| C6 Hexagonal architecture | In scope, built as part of Part B (D1) |

Order: C3 → C4 → C1, then C5 if there is time. C2 and C6 are checked as Part B is built.

### C1. Containerization
- [x] Multi-stage `Dockerfile` (build stage, then a small runtime image) (`golang:1.27` build, `gcr.io/distroless/static-debian12:nonroot` runtime, about 20 MB; exec-form entrypoint so the binary receives SIGTERM; `.dockerignore` keeps `.env` out of the build context)
- [x] `docker-compose.yml` with `api` and `mongo` services, an env file, a Mongo volume, and a health check (health check on `mongo`, `api` waits for it with `condition: service_healthy`; MongoDB is not published to the host; `API_PORT` picks the host port)
- [x] `stop_grace_period: 20s` on the `api` service: `docker stop` sends SIGTERM and kills after 10s by default, which is shorter than the 15s shutdown timeout (C4). Checked: `docker compose stop api` shut down gracefully in about 1s with exit code 0 and the full shutdown log sequence
- [x] `.env.example` (the `JWT_SECRET` placeholder is deliberately shorter than 32 bytes, so the API refuses to start until it is replaced: checked, exit code 1 with a clear message)

**Verify**
- [x] `docker compose up` brings up the API and Mongo, and the sample requests work against it. Checked 2026-10-26: every README sample gave the documented response, including both 409s; users survived a full stop and start (volume). While testing I dropped the database with the API running, which also removes the unique index until the next start (the API creates it at startup); the README now says to restart the API after dropping the collection

### C2. Abstraction (interfaces)
- [x] Repository, hasher and token issuer are all accessed through interfaces in `port/` (mostly covered by B2 and B3) Checked 2026-10-26: `port` defines `UserRepository`, `PasswordHasher`, `TokenIssuer` and `TokenValidator`; `AuthService` and `UserService` hold only those interfaces

**Verify**
- [x] Services and handlers do not import the Mongo driver package Checked 2026-10-26: of all non-test packages, only `internal/adapter/mongo` imports `go.mongodb.org/mongo-driver/v2`

### C3. Validation
Rules decided in D11:
- [x] `name` required, trimmed, 1–100 characters
- [x] `email` required and a valid format (`net/mail.ParseAddress`, bare address only: `Alice <a@b.com>` is rejected)
- [x] `email` at most 254 bytes (not characters), checked on the normalized (trimmed, lowercased) value (D11). Register uses it now; PATCH must use the same `validateEmail` (B4)
- [x] `password` required, at least 8 characters and at most 72 bytes (D21), on register. No composition rules (uppercase, digit, symbol)
- [x] Login: `email` and `password` required (format and length are not checked, so a login never reveals the rules)
- [x] PATCH needs at least one field, and each field present is validated Checked 2026-10-26: `TestUserUpdateErrors`, `TestUpdateUserErrors` (`{}`, null fields, blank name, invalid email, email over 254 bytes)
- [x] Unknown fields, malformed JSON, trailing JSON and oversized bodies are handled by the shared decode helper in B4 (D11, D17)
- [x] Return 400 with a clear message Checked 2026-10-26: handler tests assert the exact message for every 400, e.g. `name is required`, `unknown field "password"`

**Verify**
- [x] Table-driven tests cover each rule Checked 2026-10-26: `TestRegisterValidation`, `TestRegisterAcceptsBoundaryValues`, `TestValidateEmailLength`, `TestLoginValidation`, `TestUserUpdateErrors`, `TestRegisterErrors`, `TestUpdateUserErrors`: every rule has a rejected case and its accepted boundary
- [x] PATCH with `{"password": "..."}` returns 400, and so does a typo such as `{"emial": "..."}` Checked 2026-10-26: `TestUpdateUserErrors/password_field` and `/typo_field`; also confirmed with curl in B8

### C4. Graceful shutdown
- [x] `signal.NotifyContext` for `SIGINT` and `SIGTERM` (in `main`; after the first signal the default is restored, so a second Ctrl+C kills the process)
- [x] On signal: `server.Shutdown` with a timeout, stop the worker, then disconnect Mongo. If the timeout expires, `srv.Close()` force-closes the remaining connections first (logged as a warning), then the worker and Mongo are still shut down and the process exits with code 1; `TestServeShutdownTimeout` checks the stuck connection is closed and fails without the fix (`serve` shuts the server down with a 15s timeout; `run` then stops the worker and waits for it, and the deferred disconnect runs last. The worker has its own context so it keeps running while requests drain)

**Verify**
- [x] Ctrl+C logs a clean shutdown sequence and exits with code 0. Checked 2026-10-26 with the real binary against `mongo:7`: SIGINT during a slow upload logged `shutdown started` → `http server stopped` → `worker stopped` → `mongo disconnected` → `shutdown complete`, exit code 0
- [x] An in-flight request completes before exit. Checked 2026-10-26 with the real binary against `mongo:7`: SIGINT during a slow upload still got its response (5.1s request, logged after `shutdown started`); also `TestServeLetsInFlightRequestFinish`, plus tests for the shutdown timeout and a server error

### C5. gRPC support (stretch goal, D10)
Decided in D23 (2026-10-26):
- Dependencies: `google.golang.org/grpc` v1.84.0, `google.golang.org/protobuf` v1.36.12 (latest stable; both fit the Go 1.26 minimum). Only `adapter/grpc` and `cmd/api` import them
- Tooling: `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` and `grpcurl` live only in the session scratchpad; nothing is installed system-wide. The generated `.pb.go` files are committed, so building and testing never needs `protoc`; the README records the tool versions and the regenerate command
- Auth mirrors REST: `CreateUser` is public like `POST /auth/register` (D2); `GetUser` needs `authorization: Bearer <token>` metadata like `GET /users/{id}`, checked by a unary interceptor that reuses `port.TokenValidator`
- Server reflection is on, so `grpcurl` works without the `.proto` file
- [x] `proto/user.proto` with the `CreateUser` and `GetUser` RPCs (package `user.v1`; `User` has no password field; `created_at` is a `google.protobuf.Timestamp`)
- [x] Generate code with `protoc` (and record the command in the README or a Makefile): `protoc` 36.2, `protoc-gen-go` v1.36.12, `protoc-gen-go-grpc` v1.6.2; output in `internal/adapter/grpc/userpb/` (committed); command and versions in the README
- [x] gRPC adapter in `adapter/grpc/`: `CreateUser` calls `AuthService.Register` (D14), and `GetUser` calls `UserService.Get`. `GetUser` maps `ErrInvalidID` to `InvalidArgument` and `ErrNotFound` to `NotFound` (D15). Also maps `ErrValidation` → `InvalidArgument`, `ErrEmailExists` → `AlreadyExists`, anything else → generic `Internal` (logged)
- [x] Optional: a unary interceptor that validates the JWT from the `authorization` metadata: done (D23). `CreateUser` is public, `GetUser` needs `authorization: Bearer <token>`; same parsing rules as the HTTP middleware. Plus a logging interceptor: one entry per call with method, code and duration
- [x] Runs alongside HTTP on a separate port (`GRPC_PORT`) and shuts down gracefully: `GRPC_PORT` (default 9090), also exposed in the Dockerfile and compose (`API_GRPC_PORT`). `serveAll` runs HTTP and gRPC together, shuts them down in parallel (so the total stays within the 20s Docker grace period) and stops both if either fails. On a gRPC shutdown timeout, `Stop` runs in the background: in grpc-go v1.84 a pending `GracefulStop` holds the server lock while waiting for a stuck handler, which made a synchronous `Stop` hang; `TestServeGRPCShutdownTimeout` caught this and now passes 200 runs in a row under `-race`
- [x] Tests that use `bufconn`: `internal/adapter/grpc` at 100% (public `CreateUser`, token rules, every status code, generic `Internal`, logging, reflection); `cmd/api` has `serveGRPC` and `serveAll` tests over real TCP

**Verify**
- [x] `grpcurl` can create and get a user. Checked 2026-10-26 against the real binary and through Docker Compose: reflection `list`/`describe`, `CreateUser` without a token, `GetUser` with a token from REST login, every error code, and a gRPC-created user read back over REST
- [x] The README has grpcurl examples (new "gRPC API" section, recorded from the running server)

### C6. Hexagonal architecture
- [x] Domain depends on nothing. Services depend only on domain and ports. Adapters depend on ports and services. `cmd` wires everything together Checked 2026-10-26: `domain` imports nothing internal; `port` → `domain`; `service` → `domain`, `port`; `adapter/http` → `domain`, `port`, `service`; `adapter/mongo` → `domain`, `port`; `adapter/auth` → `port`; `worker` → nothing internal
- [x] The README has a short architecture section explaining the layers

**Verify**
- [x] `go list -deps` on `domain` and `service` shows no HTTP, Mongo or gRPC packages Checked 2026-10-26: no `net/http`, Mongo driver, gRPC, JWT or bcrypt packages in their transitive dependencies

---

## Final checklist before submitting

Run 2026-10-26 on a fresh clone of `c2a70fd`.

- [x] All of Part B is ticked (0 open items)
- [x] `gofmt -l .` produces no output, and `go vet ./...` is clean
- [x] `go test -race -cover ./...` passes (`service` 100%, `worker` 100%, `adapter/http` 97.1%, `adapter/auth` 96.6%, `cmd/api` 41.1%, `adapter/mongo` 11.4% by design, D5)
- [x] `GOTOOLCHAIN=go1.26.0 go test -race ./...` passes, so the Go 1.26 minimum is real (D22; checked 2026-10-26 after the switch)
- [x] No secrets are committed (`.env` is git-ignored): whole history scanned. The only env file ever committed is `.env.example`; the only `JWT_SECRET` values are the `replace-me` placeholder and the README's `openssl` command; all 63 base64 strings are `go.sum` checksums; the one JWT is the expired README sample, signed with a throwaway secret that was never committed
- [x] README tested from a fresh clone: both the Docker Compose path (build, start, try-it, duplicate → 409, graceful stop with exit 0) and the local Go path (exports, run, try-it, SIGINT shutdown sequence). Port 8090/8089 because 8080 is taken on this machine
- [x] Assumptions in the README match what was implemented: 34 stated values and rules (limits, timeouts, defaults, JWT rules, Docker settings, versions, the 6 routes) checked against the source; all match

### Optional enhancement (only after the final checklist passes)
- [ ] Reconsider MongoDB integration tests (D5) if time remains: for example a separate opt-in run against Docker, so the adapter checks done manually in B2 and B8 become repeatable. This needs a new decision first; it must not delay B9, C4 or C1
