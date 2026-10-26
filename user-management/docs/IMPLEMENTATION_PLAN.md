# User Management API — Implementation Plan

> Source of truth: [`TEST-ASSIGNMENT.md`](../../TEST-ASSIGNMENT.md), section "User Management API" (the original brief). If anything here conflicts with the brief, the brief wins.
> Summary used for this plan: [`requirements.md`](./requirements.md)
> Scope: plan only, no code. Tick each `[ ]` → `[x]` as it is done.
> Each phase ends with a **Verify** checklist that has to pass before the phase counts as done.

---

## Part A — Assumptions

These are proposed answers to the open points in `requirements.md` §5. Confirm or change them before you start Part B. Each one also goes into the README's "Assumptions & Design Decisions" section (B9).

> **Status:** Items marked ✅ were decided one by one (D1–D17). Unticked items in Part A are proposed defaults that were not discussed individually. Accept them, or raise objections, before starting B0.
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
| D5 | Mocking strategy | Hand-written in-memory fakes that implement the port interfaces. No mock generator, no mocking library | No (README note) |
| D6 | JWT details | Access token only, default TTL 1h (env), claims `sub`, `iss`, `iat`, `exp` (no email), `iss` validated. No refresh token | No (README JWT guide) |
| D7 | Update semantics | `PATCH /users/{id}`, partial update of `name` and/or `email`, at least one field. Password can't be changed | No (README note) |
| D8 | List users | `GET /users` returns every user as a plain JSON array. No pagination | No (README note) |
| D9 | Delete semantics | Hard delete. `204` on success, `404` when the ID doesn't exist | No |
| D10 | Bonus scope | Do C1, C3, C4 (C2 and C6 come with D1/D5). C5 gRPC is a stretch goal, only after Part B and the final checklist pass | No |
| D11 | Validation rules | Length and format rules as in C3 (no password composition rules). Unknown JSON fields are rejected with 400 | No (README note) |
| D12 | Go module | `go.mod` in `user-management/`, module path `github.com/jingjoeh/test-assignment/user-management` | No |
| D13 | MongoDB driver | Official driver v2 (`go.mongodb.org/mongo-driver/v2`) | No |
| D14 | User creation ownership | `AuthService.Register` is the only place that creates users. There is no `UserService.Create`, and services don't call each other | No (mention in ADR-001) |
| D15 | ID handling | Malformed ObjectID → 400 (`ErrInvalidID`). Well-formed ID that doesn't exist → 404 (`ErrNotFound`) | No (README note) |
| D16 | Worker testability | The worker reads ticks from an injected channel, so tests drive it by hand with no sleeps and no real ticker | No |
| D17 | Security hardening | Required JWT claims, `JWT_SECRET` of at least 32 bytes, HTTP server timeouts, 1 MiB request-body limit, trailing JSON rejected | No (README note) |

### A0. Architecture — ✅ Decided (D1)
- [x] Use hexagonal architecture (ports and adapters) from the first commit, not as a later refactor
- [x] Layers: `domain` (entities and errors, no dependencies) → `port` (interfaces) → `service` (application logic) → `adapter/http`, `adapter/mongo` (and later `adapter/grpc`) → `cmd/api` (wiring)
- [x] Keep it to these core layers only, with no extra sub-layers, to avoid over-engineering
- [x] Explain the reasoning in the README's architecture section (C6)

### A1. Tech stack
- [ ] Go 1.23+, using Go modules (D4 needs at least 1.22 for `ServeMux` patterns)
- [x] HTTP router: the standard library `net/http` (`ServeMux` with method and path patterns), so there is no framework dependency. Middleware is `func(http.Handler) http.Handler`, and small JSON helpers are written in `adapter/http`. ✅ Decided (D4)
- [ ] MongoDB 7.x server
- [x] Official driver v2: `go.mongodb.org/mongo-driver/v2`. Only `adapter/mongo` imports it (D1). ✅ Decided (D13)
- [ ] JWT: `github.com/golang-jwt/jwt/v5`
- [ ] Password hashing: `golang.org/x/crypto/bcrypt`
- [ ] Logging: the standard library `log/slog`, with JSON output to stdout
- [ ] Configuration through environment variables: `PORT`, `MONGO_URI`, `MONGO_DB`, `JWT_SECRET` (at least 32 bytes, D17), `JWT_TTL` (default `1h`, D6), `JWT_ISSUER` (D6)

### A2. User model
- [ ] `ID` is a MongoDB `ObjectID`, exposed in the API as a hex string
- [ ] `Email` is trimmed, lowercased and stored that way; uniqueness comes from a unique index on `email`
- [ ] `Password` is a bcrypt hash at the default cost. It is never returned in any response
- [ ] `CreatedAt` is stored in UTC and returned as RFC 3339

### A3. Authentication and authorization
- [x] `POST /auth/register` is public and creates a user (name, email, password). ✅ Decided (D2)
- [x] `AuthService.Register` is the sole owner of user creation: it validates input, normalizes the email, hashes the password, builds the `domain.User` (including `CreatedAt`) and calls `UserRepository.Create`. There is no `UserService.Create`, and no service calls another service. ✅ Decided (D14)
- [ ] `POST /auth/login` is public. It takes email and password and returns `{ "token": "..." }`
- [x] There is no separate `POST /users`. Registration is the only way to create a user and covers the brief's "Create a new user" (req. 3), because a protected create-user endpoint has no clear use case. ✅ Decided (D2)
- [x] Known risk: a grader checking req. 3 item by item may look for a create endpoint under `/users`. Mitigation: the README maps "Create a new user" → `POST /auth/register` explicitly (B9). ✅ Decided (D2)
- [ ] Every `/users` endpoint requires `Authorization: Bearer <token>`
- [x] JWT claims are `sub` (user ID), `iss`, `iat` and `exp`. There is no `email` claim, so a token can't carry a stale email after an update. ✅ Decided (D6)
- [x] The default TTL is 1h, configurable through `JWT_TTL`. ✅ Decided (D6)
- [x] `iss` is set from `JWT_ISSUER` and checked on validation. A token with the wrong issuer is rejected. ✅ Decided (D6)
- [x] All four claims are required on validation. A token missing `sub`, `iss`, `iat` or `exp`, or with an empty `sub`, is rejected with 401 even if its signature is valid. ✅ Decided (D17)
- [x] There is no refresh token and no revocation. The README lists both under "Known limitations". ✅ Decided (D6)
- [ ] Signing uses HS256 only. Tokens signed with any other `alg` are rejected
- [x] The service refuses to start if `JWT_SECRET` is shorter than 32 bytes (256 bits, the HS256 key size), which also covers an empty or missing secret. `.env.example` shows a placeholder and how to generate a real one (e.g. `openssl rand -base64 32`). ✅ Decided (D17)
- [x] Authorization: any authenticated user can manage any user. There is no ownership check and there are no roles; this matches the brief, which only asks to protect endpoints with JWT. ✅ Decided (D3)
- [x] Known limitation: one user can update or delete another user. The README lists this under "Known limitations" and names ownership checks or roles as possible future work. ✅ Decided (D3)

### A4. REST API contract

| Method | Path | Auth | Success |
| --- | --- | --- | --- |
| POST | `/auth/register` | public | 201 + user |
| POST | `/auth/login` | public | 200 + token |
| GET | `/users` | JWT | 200 + list |
| GET | `/users/{id}` | JWT | 200 + user |
| PATCH | `/users/{id}` | JWT | 200 + user |
| DELETE | `/users/{id}` | JWT | 204 |

- [x] PATCH is a partial update: `name`, `email` or both. At least one field is required; an empty body or `{}` returns 400. ✅ Decided (D7)
- [x] The request DTO uses pointer fields (`*string`) to tell "field not sent" apart from "field sent as empty". ✅ Decided (D7)
- [x] The password cannot be changed through this endpoint or any other. ✅ Decided (D7)
- [x] Unknown fields in the body (e.g. `password` on PATCH) are rejected with 400. ✅ Decided (D11)
- [x] `GET /users` returns every user as a plain JSON array (`[...]`, `[]` when empty) with no pagination, as the brief says ("list all"). ✅ Decided (D8)
- [x] Pagination is listed in the README under "Known limitations" as future work. ✅ Decided (D8)
- [x] Delete is a hard delete: the document is removed from the collection, and there is no `DeletedAt` field. ✅ Decided (D9)
- [x] `DELETE /users/{id}` returns `204` on success and `404` when the ID doesn't exist, including a second delete of the same ID. ✅ Decided (D9)
- [x] For every `/users/{id}` route (GET, PATCH, DELETE), a malformed ObjectID returns `400`, and a well-formed ID with no matching user returns `404`. ✅ Decided (D15)
- [x] Request bodies are limited to 1 MiB; a larger body returns `413`. ✅ Decided (D17)
- [x] A body with extra data after the first JSON value (e.g. `{"name":"a"}{"name":"b"}`) returns `400`. ✅ Decided (D17)
- [ ] Error format is `{ "error": "<message>" }`. Status codes: 400 validation, malformed JSON or malformed ID; 401 auth; 404 not found; 409 duplicate email; 413 body too large; 500 internal (generic message, details only in the log)

### A5. Middleware and background task
- [ ] The logging middleware logs method, path, status code and duration. Status code goes beyond what the brief asks for
- [ ] The user-count goroutine uses a `time.Ticker` at 10s, so the first log appears 10s after start. Errors are logged and it keeps running. It stops when the root context is cancelled
- [x] The worker doesn't create its own ticker. It receives ticks from an injected `<-chan time.Time`: `main` passes `ticker.C`, and tests pass a channel they control. ✅ Decided (D16)

### A5a. HTTP server hardening — ✅ Decided (D17)
- [x] `http.Server` sets `ReadHeaderTimeout` 5s, `ReadTimeout` 10s, `WriteTimeout` 10s and `IdleTimeout` 60s. No server runs with the zero-value (unlimited) timeouts
- [x] Values are constants in `cmd/api`, not env vars, since the brief doesn't need them configurable

### A6. Testing
- [x] Unit tests use only the standard library `testing` package, with hand-written fakes and no mocking library or code generation. ✅ Decided (D5)
- [x] Mocking happens at the repository interface, so service and handler tests don't need a real MongoDB. ✅ Decided (D5)
- [x] The fake repository behaves like the real one where it matters (duplicate email → `ErrEmailExists`, missing ID → `ErrNotFound`) and has a settable error field so tests can force failures. ✅ Decided (D5)
- [x] Integration tests against a real MongoDB are out of scope. ✅ Decided (D5)
- [ ] Coverage target is at least 70% for the domain, service and handler packages
- [x] Because mandatory requirement 7 needs MongoDB mocked, the interface abstraction (Bonus "Abstraction") gets built as part of the mandatory work. ✅ Decided (D1, D5, D10)

### A7. Repository layout
- [x] All code and documentation for this part live under `user-management/`. That folder has its own `README.md` and `go.mod`. ✅ Decided (D12)
- [x] The module path is based on the GitHub repo rather than a short local name. ✅ Decided (D12)
- [x] Module path: `github.com/jingjoeh/test-assignment/user-management`. The GitHub repo was renamed from the misspelled `test-assingment` before any code was written. ✅ Decided (D12)

---

## Part B — Mandatory Requirements

### B0. Project skeleton
- [ ] `go mod init github.com/jingjoeh/test-assignment/user-management` inside `user-management/` (D12)
- [ ] Create the folder structure (the hexagonal layout is also bonus C6):
  ```
  user-management/
  ├── cmd/api/                 # main: wiring, config, server start
  ├── internal/
  │   ├── domain/              # User entity, domain errors
  │   ├── port/                # interfaces: UserRepository, PasswordHasher, TokenIssuer
  │   ├── service/             # AuthService (register, login), UserService (get, list, update, delete)
  │   ├── adapter/
  │   │   ├── http/            # handlers, middleware, DTOs
  │   │   └── mongo/           # repository implementation
  │   └── worker/              # background user counter
  └── docs/
  ```
- [ ] Config loader that reads the env vars from A1 and fails fast when a required value is missing or `JWT_SECRET` is shorter than 32 bytes (D17)

**Verify**
- [ ] Config tests: missing required var fails, 31-byte secret fails, 32-byte secret passes, `JWT_TTL` defaults to `1h` and rejects an invalid duration
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean

### B1. User model (req. 1)
- [ ] `domain.User` with `ID`, `Name`, `Email`, `PasswordHash`, `CreatedAt`
- [ ] Domain errors: `ErrNotFound`, `ErrInvalidID` (D15), `ErrEmailExists`, `ErrInvalidCredentials`, `ErrValidation`

**Verify**
- [ ] All five fields from the brief are present
- [ ] The JSON response DTO has no password field

### B2. MongoDB integration (req. 4)
- [ ] `port.UserRepository` interface with `Create`, `GetByID`, `GetByEmail`, `List`, `Update`, `Delete`, `Count`
- [ ] Mongo adapter that implements the interface with the official driver
- [ ] Create a unique index on `email` at startup
- [ ] Map duplicate-key errors to `ErrEmailExists` and `ErrNoDocuments` to `ErrNotFound`
- [ ] Map an invalid ObjectID hex string to `ErrInvalidID` (→ 400), and a valid ID with no document to `ErrNotFound` (→ 404) (D15). Parsing stays in `adapter/mongo`, so the domain keeps IDs as plain strings
- [ ] Use a context with a timeout on every DB call
- [ ] Connect at startup with a ping check, and disconnect on shutdown

**Verify**
- [ ] When run against a local MongoDB, a created user can be read back
- [ ] Inserting a duplicate email returns `ErrEmailExists`
- [ ] `GetByID("not-an-id")` returns `ErrInvalidID`, and `GetByID` with a valid but unused ObjectID returns `ErrNotFound`

### B3. Authentication and JWT (req. 2)
- [ ] Password hasher (bcrypt) behind an interface
- [ ] Token issuer and validator (HS256 only, secret from config, `exp` and `iss` enforced, D6; `sub`, `iss`, `iat`, `exp` all required and `sub` non-empty, D17)
- [ ] `AuthService.Register` (sole owner of user creation, D14): validate input, normalize the email, hash the password, set `CreatedAt`, call `UserRepository.Create` directly. It doesn't call `UserService`
- [ ] `AuthService.Login`: look up by email, compare the hash, issue a token. Use the same error for an unknown email and a wrong password
- [ ] JWT middleware: parse the `Bearer` header, validate the token, put the user ID in the request context, and return 401 on any failure

**Verify**
- [ ] Login with valid credentials returns a token that decodes with `alg: HS256`
- [ ] Without a token, a protected route returns 401
- [ ] An expired, tampered, wrong-alg or wrong-issuer token returns 401
- [ ] A correctly signed token missing `sub`, `iss`, `iat` or `exp`, or with an empty `sub`, returns 401 (D17)
- [ ] A wrong password and an unknown email return the same 401 response

### B4. User operations (req. 3)
- [ ] `UserService`: `Get`, `List`, `Update` (name and/or email), `Delete`. No `Create`: creation belongs to `AuthService.Register` (D14)
- [ ] Handlers and routes for `/users` as in A4, all wrapped in the JWT middleware
- [ ] Map domain errors to HTTP status codes in one place (`ErrInvalidID` → 400, `ErrNotFound` → 404, D15)
- [ ] One shared JSON decode helper used by every handler that reads a body:
  - [ ] Wraps the body in `http.MaxBytesReader` (1 MiB); exceeding it returns 413 (D17)
  - [ ] Uses `DisallowUnknownFields`, so unknown fields return 400 (D11)
  - [ ] After decoding one value, checks that the rest of the body is empty (a second `Decode` must return `io.EOF`); otherwise 400 (D17)
  - [ ] Malformed or empty JSON returns 400

**Verify**
- [ ] Each of the five operations works end-to-end with curl (create = `POST /auth/register`)
- [ ] Updating to an email that already exists returns 409
- [ ] PATCH with only `name`, only `email`, or both updates just those fields; PATCH with `{}` returns 400 (D7)
- [ ] Get, update or delete on a well-formed but missing ID returns 404, and on a malformed ID returns 400 (D15)
- [ ] A body over 1 MiB returns 413, and a body with trailing JSON returns 400 (D17)
- [ ] No response contains a password or hash

### B5. Logging middleware (req. 5)
- [ ] Middleware that records start time, wraps `ResponseWriter` to capture the status, and logs method, path, status and duration
- [ ] Applied to every route, public ones included

**Verify**
- [ ] Every request produces one log line containing method, path and duration

### B6. Concurrency task (req. 6)
- [ ] Worker that takes a context, a tick channel (`<-chan time.Time`), a `Count` dependency (an interface) and a `*slog.Logger` (D16)
- [ ] On each tick it calls `Count` and logs `total_users=<n>`; on error it logs the error and waits for the next tick
- [ ] `Run` returns when the context is cancelled or the tick channel is closed
- [ ] `main` creates the 10s `time.Ticker`, passes `ticker.C`, runs the worker in its own goroutine and stops the ticker on shutdown

**Verify**
- [ ] Manual smoke check: after the app runs for about 30s, the log has about 3 user-count lines
- [ ] Manual smoke check: registering a user changes the count in the next tick
- [ ] The goroutine exits when the context is cancelled (checked by a deterministic test, B7)

### B7. Testing (req. 7)
- [ ] Fake `UserRepository` (an in-memory map with a settable error for failure cases, D5) for service and handler tests
- [ ] Fakes for `PasswordHasher` and `TokenIssuer` where a test needs to isolate the service from bcrypt or JWT
- [ ] `AuthService` tests:
  - [ ] Register: success, duplicate email → `ErrEmailExists`, duplicate email differing only in case or surrounding spaces → `ErrEmailExists`, invalid input → `ErrValidation` with no repository call
  - [ ] Register: the stored user has a bcrypt hash (not the plain password), a lowercased email and a non-zero UTC `CreatedAt`
  - [ ] Register: a hasher error or repository error is returned and nothing is half-created
  - [ ] Login: success returns a token whose `sub` is the user ID; email lookup is case-insensitive
  - [ ] Login: wrong password and unknown email both return `ErrInvalidCredentials`; a repository error is not turned into `ErrInvalidCredentials`
- [ ] `UserService` tests:
  - [ ] Get: success, `ErrNotFound`, `ErrInvalidID`
  - [ ] List: empty repository returns an empty (non-nil) slice; several users are all returned
  - [ ] Update: name only, email only, both, no fields → `ErrValidation`, duplicate email → `ErrEmailExists`, updating to the user's own current email succeeds, email normalized, `ErrNotFound`, `ErrInvalidID`
  - [ ] Update and Delete leave `CreatedAt` and the password hash unchanged
  - [ ] Delete: success, `ErrNotFound`, `ErrInvalidID`
  - [ ] Every method passes repository errors through
- [ ] Token tests: issue then validate, expired token, tampered token, wrong alg (including `none`), wrong issuer, wrong secret, and each required claim missing (D17)
- [ ] Middleware tests:
  - [ ] JWT: missing header, header without `Bearer `, empty token, invalid token, valid token puts the user ID in the request context
  - [ ] Logging: method, path, status and duration are logged, including when the handler never calls `WriteHeader` (status 200)
- [ ] Handler tests with `httptest`, for every route:
  - [ ] Success status and JSON body; `Content-Type: application/json`; no `password` or hash field in any response
  - [ ] `POST /auth/register`: 201, 400 (validation, unknown field, malformed JSON, trailing JSON), 409, 413
  - [ ] `POST /auth/login`: 200 with token, 400 (missing field, malformed JSON), 401 with the same body for wrong password and unknown email
  - [ ] `GET /users`: 200 with `[]` when empty, 200 with users
  - [ ] `GET /users/{id}`: 200, 400 malformed ID, 404 missing ID (D15)
  - [ ] `PATCH /users/{id}`: 200, 400 (`{}`, unknown field such as `password`, invalid email, trailing JSON), 400 malformed ID, 404, 409, 413
  - [ ] `DELETE /users/{id}`: 204 with empty body, 400 malformed ID, 404 missing ID, 404 on second delete
  - [ ] Every `/users` route returns 401 without a valid token
  - [ ] A repository failure returns 500 with a generic message that doesn't leak the internal error
- [ ] Update tests: name only, email only, both, empty body, duplicate email, missing ID (D7)
- [ ] Delete tests: success returns 204, missing ID returns 404, deleting twice returns 404 the second time, and a deleted email can register again (D9)
- [ ] Worker tests, deterministic with no `time.Sleep` and no real ticker (D16):
  - [ ] The test owns an unbuffered tick channel and runs `Run` in a goroutine with a `done` channel
  - [ ] Each unbuffered send blocks until the worker has taken the tick, so the test controls exactly how many ticks happen
  - [ ] After the last tick, the test cancels the context and waits for `done`; because `Run` has returned, all log writes are finished and the captured log (a `bytes.Buffer` behind the `slog` handler) can be asserted
  - [ ] Cases: N ticks → N `total_users` lines with the fake counter's values; counter error → error logged and the next tick still counts; cancel → `Run` returns; closed tick channel → `Run` returns
  - [ ] Waiting on `done` uses a generous `select` timeout only as a failsafe to fail the test, never to drive it

**Verify**
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `go test -cover ./...` meets the A6 target

### B8. Wiring (`cmd/api`)
- [ ] Load config, then connect Mongo, then build adapters, services, handlers and router, then start the worker, then start the HTTP server
- [ ] Build `http.Server` with the timeouts from A5a (D17), never `http.ListenAndServe` with the default server

**Verify**
- [ ] `go run ./cmd/api` starts and serves requests against a local MongoDB
- [ ] `go run ./cmd/api` with a `JWT_SECRET` under 32 bytes exits with a clear error before connecting to Mongo

### B9. Deliverables (documentation)
- [ ] `user-management/README.md`
  - [ ] Prerequisites and environment variables
  - [ ] Setup and run instructions (local and Docker if C1 is done)
  - [ ] How to run the tests
- [ ] JWT guide: how to register or log in to get a token, how to send it in the `Authorization` header, the required claims, and how to generate a `JWT_SECRET` of at least 32 bytes (D17)
- [ ] Sample requests and responses (curl) for every endpoint, including error examples
- [ ] Requirement-to-endpoint table that maps each brief item to its endpoint, including "Create a new user" → `POST /auth/register` (D2)
- [ ] Assumptions & Design Decisions section, copied from Part A and kept up to date
- [ ] Known limitations section, including the authorization limitation from D3, no refresh token or revocation from D6, and no pagination from D8

**Verify**
- [ ] Following the README from a fresh clone, the app runs and the sample requests work as documented

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
- [ ] Multi-stage `Dockerfile` (build stage, then a small runtime image)
- [ ] `docker-compose.yml` with `api` and `mongo` services, an env file, a Mongo volume, and a health check
- [ ] `.env.example`

**Verify**
- [ ] `docker compose up` brings up the API and Mongo, and the sample requests work against it

### C2. Abstraction (interfaces)
- [ ] Repository, hasher and token issuer are all accessed through interfaces in `port/` (mostly covered by B2 and B3)

**Verify**
- [ ] Services and handlers do not import the Mongo driver package

### C3. Validation
Rules decided in D11:
- [ ] `name` required, trimmed, 1–100 characters
- [ ] `email` required and a valid format (`net/mail.ParseAddress`)
- [ ] `password` required, at least 8 characters, on register. No composition rules (uppercase, digit, symbol)
- [ ] Login: `email` and `password` required (format and length are not checked, so a login never reveals the rules)
- [ ] PATCH needs at least one field, and each field present is validated
- [ ] Unknown fields, malformed JSON, trailing JSON and oversized bodies are handled by the shared decode helper in B4 (D11, D17)
- [ ] Return 400 with a clear message

**Verify**
- [ ] Table-driven tests cover each rule
- [ ] PATCH with `{"password": "..."}` returns 400, and so does a typo such as `{"emial": "..."}`

### C4. Graceful shutdown
- [ ] `signal.NotifyContext` for `SIGINT` and `SIGTERM`
- [ ] On signal: `server.Shutdown` with a timeout, stop the worker, then disconnect Mongo

**Verify**
- [ ] Ctrl+C logs a clean shutdown sequence and exits with code 0
- [ ] An in-flight request completes before exit

### C5. gRPC support (stretch goal, D10)
- [ ] `proto/user.proto` with the `CreateUser` and `GetUser` RPCs
- [ ] Generate code with `protoc` (and record the command in the README or a Makefile)
- [ ] gRPC adapter in `adapter/grpc/`: `CreateUser` calls `AuthService.Register` (D14), and `GetUser` calls `UserService.Get`. `GetUser` maps `ErrInvalidID` to `InvalidArgument` and `ErrNotFound` to `NotFound` (D15)
- [ ] Optional: a unary interceptor that validates the JWT from the `authorization` metadata
- [ ] Runs alongside HTTP on a separate port (`GRPC_PORT`) and shuts down gracefully
- [ ] Tests that use `bufconn`

**Verify**
- [ ] `grpcurl` can create and get a user
- [ ] The README has grpcurl examples

### C6. Hexagonal architecture
- [ ] Domain depends on nothing. Services depend only on domain and ports. Adapters depend on ports and services. `cmd` wires everything together
- [ ] The README has a short architecture section explaining the layers

**Verify**
- [ ] `go list -deps` on `domain` and `service` shows no HTTP, Mongo or gRPC packages

---

## Final checklist before submitting

- [ ] All of Part B is ticked
- [ ] `gofmt -l .` produces no output, and `go vet ./...` is clean
- [ ] `go test -race -cover ./...` passes
- [ ] No secrets are committed (`.env` is git-ignored)
- [ ] README tested from a fresh clone
- [ ] Assumptions in the README match what was implemented
