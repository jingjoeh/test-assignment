# User Management API

A RESTful API in Go for managing users, with MongoDB for persistence and JWT (HS256) for authentication. It is the "User Management API" part of [`TEST-ASSIGNMENT.md`](../TEST-ASSIGNMENT.md).

- Go standard library HTTP server (`net/http`), no web framework
- MongoDB through the official Go driver (v2)
- Passwords hashed with bcrypt; JWT access tokens signed with HS256
- Request logging middleware and a background goroutine that logs the user count every 10 seconds
- Hexagonal architecture (ports and adapters), unit tests with hand-written fakes

## Contents

- [Requirements coverage](#requirements-coverage)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [API reference](#api-reference)
- [JWT guide](#jwt-guide)
- [Sample requests and responses](#sample-requests-and-responses)
- [Testing](#testing)
- [Architecture](#architecture)
- [Assumptions and design decisions](#assumptions-and-design-decisions)
- [Known limitations](#known-limitations)
- [Bonus features](#bonus-features)

## Requirements coverage

How each item in the brief maps to this implementation.

| Brief | Implementation |
| --- | --- |
| 1. User model: `ID`, `Name`, `Email` (unique), `Password` (hashed), `CreatedAt` | `internal/domain/user.go`; unique index on `email`; bcrypt hash; `createdAt` in UTC |
| 2. User registration | `POST /auth/register` |
| 2. Authentication that returns a JWT | `POST /auth/login` → `{"token": "..."}` |
| 2. Protect endpoints with JWT, validate via middleware, HS256 | Every `/users` route goes through `RequireAuth` (`internal/adapter/http/middleware_auth.go`) |
| 3. Create a new user | `POST /auth/register` (see [why there is no `POST /users`](#assumptions-and-design-decisions)) |
| 3. Fetch a user by ID | `GET /users/{id}` |
| 3. List all users | `GET /users` |
| 3. Update a user's name or email | `PATCH /users/{id}` |
| 3. Delete a user | `DELETE /users/{id}` |
| 4. Official Go MongoDB driver, persist and retrieve users | `go.mongodb.org/mongo-driver/v2` in `internal/adapter/mongo` |
| 5. Logging middleware: method, path, execution time | `internal/adapter/http/middleware_logging.go` (also logs the status code) |
| 6. Goroutine logging the user count every 10 seconds | `internal/worker/user_count.go`, started from `cmd/api` |
| 7. Unit tests with `testing`, MongoDB mocked | `go test ./...`; fakes in `internal/fake` (see [Testing](#testing)) |

## Quick start

### Prerequisites

- Go 1.26 or newer. With an older Go (1.21+), the `go` command downloads 1.26 automatically.
- Docker, to run MongoDB 7 locally (or any MongoDB 7 you can reach).

### 1. Start MongoDB

```sh
docker run -d --name um-mongo -p 127.0.0.1:27017:27017 mongo:7
```

### 2. Configure

All configuration comes from environment variables (see [Configuration](#configuration)). `JWT_SECRET` must be at least 32 bytes; generate one:

```sh
export MONGO_URI=mongodb://127.0.0.1:27017
export MONGO_DB=user_management
export JWT_ISSUER=user-management
export JWT_SECRET="$(openssl rand -base64 32)"
```

### 3. Run

```sh
cd user-management
go run ./cmd/api
```

The server listens on `:8080` (set `PORT` to change it). Logs are JSON on stdout:

```json
{"time":"2026-10-26T16:40:38.762763+07:00","level":"INFO","msg":"server listening","addr":":8080","mongo_db":"user_management"}
{"time":"2026-10-26T16:40:48.767093+07:00","level":"INFO","msg":"user count","total_users":0}
{"time":"2026-10-26T16:40:51.713695+07:00","level":"INFO","msg":"http request","method":"POST","path":"/auth/register","status":201,"duration_ms":49.083}
```

Stop it with `Ctrl+C` (or `SIGTERM`, which is what `docker stop` sends). Shutdown is graceful: the server stops accepting connections, lets requests already in progress finish (up to 15 seconds), then stops the user-count worker and disconnects from MongoDB:

```json
{"level":"INFO","msg":"shutdown started","timeout":"15s"}
{"level":"INFO","msg":"http server stopped"}
{"level":"INFO","msg":"worker stopped"}
{"level":"INFO","msg":"mongo disconnected"}
{"level":"INFO","msg":"shutdown complete"}
```

If requests are still running after 15 seconds, their connections are closed, a `shutdown timed out, closing connections` warning is logged, the worker and MongoDB are still shut down, and the process exits with code 1. A second `Ctrl+C` during shutdown stops the process immediately.

### 4. Try it

```sh
curl -s -X POST localhost:8080/auth/register -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"s3cret-pass"}'

TOKEN=$(curl -s -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"s3cret-pass"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

curl -s localhost:8080/users -H "Authorization: Bearer $TOKEN"
```

## Configuration

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `MONGO_URI` | yes | | e.g. `mongodb://127.0.0.1:27017` |
| `MONGO_DB` | yes | | Database name; users are stored in its `users` collection |
| `JWT_SECRET` | yes | | HMAC key for HS256. At least 32 bytes (256 bits), or the server refuses to start |
| `JWT_ISSUER` | yes | | Written to the `iss` claim and required on every token |
| `JWT_TTL` | no | `1h` | Token lifetime, a Go duration such as `15m` or `2h`; must be positive |
| `PORT` | no | `8080` | HTTP port |

Configuration is validated before anything else runs, and every problem is reported in one error. For example, a short secret exits immediately without touching MongoDB:

```json
{"level":"ERROR","msg":"server stopped","error":"invalid configuration: JWT_SECRET must be at least 32 bytes, got 5"}
```

The HTTP server has fixed timeouts: 5s to read headers, 10s to read a request, 10s to write a response, and 60s for idle connections.

## API reference

| Method | Path | Auth | Success | Body |
| --- | --- | --- | --- | --- |
| `POST` | `/auth/register` | public | `201` user | `{"name", "email", "password"}` |
| `POST` | `/auth/login` | public | `200` `{"token"}` | `{"email", "password"}` |
| `GET` | `/users` | Bearer | `200` array of users | |
| `GET` | `/users/{id}` | Bearer | `200` user | |
| `PATCH` | `/users/{id}` | Bearer | `200` user | `{"name"}`, `{"email"}` or both |
| `DELETE` | `/users/{id}` | Bearer | `204`, no body | |

A user is returned as:

```json
{"id": "6adf24024810ac71427b7f42", "name": "Alice", "email": "alice@example.com", "createdAt": "2026-10-26T09:57:22.561Z"}
```

The password and its hash are never returned.

### Request rules

- **Name:** required, trimmed, 1–100 characters.
- **Email:** required, a plain address (not `Alice <a@b.com>`), at most 254 bytes. It is trimmed and lowercased before it is validated and stored, so `Alice@Example.com` and `alice@example.com` are the same account.
- **Password:** required on registration, at least 8 characters and at most 72 bytes (bcrypt's limit). No composition rules.
- **Login** only checks that `email` and `password` are present, so it never reveals the registration rules.
- **PATCH** needs at least one of `name` or `email`; a field that isn't sent is left unchanged. The password can't be changed.
- **Bodies** must be a single JSON object of at most 1 MiB. Unknown fields (including `password` on PATCH), malformed JSON and anything after the first JSON value are rejected.

### Errors

Every error from the API is JSON with `Content-Type: application/json`:

```json
{"error": "user not found"}
```

The one exception is a request so malformed at the HTTP level (for example an invalid request line) that Go's server rejects it before it reaches the API; that gets the server's plain-text `400 Bad Request`.

| Status | When |
| --- | --- |
| `400` | Validation failure, malformed or empty JSON, unknown field, trailing data, malformed user ID |
| `401` | Missing or invalid token; wrong email or password on login |
| `404` | User not found (well-formed ID); unknown route |
| `405` | Known route, wrong method (the `Allow` header lists the valid methods) |
| `409` | Email already in use |
| `413` | Request body over 1 MiB |
| `500` | Unexpected failure; the client gets `"internal server error"` and the details go to the log only |

## JWT guide

### Getting a token

1. Register once with `POST /auth/register`.
2. Log in with `POST /auth/login` and the same email and password. The response is `{"token": "<jwt>"}`.

### Using a token

Send it in the `Authorization` header on every `/users` request:

```
Authorization: Bearer <jwt>
```

The scheme is case-insensitive (`bearer` works) and extra whitespace is ignored, but the header must be exactly the scheme and the token. A missing, malformed, expired or otherwise invalid token gets `401` with a `WWW-Authenticate: Bearer` header.

### What's inside

Tokens are signed with HS256 using `JWT_SECRET`. The header is `{"alg":"HS256","typ":"JWT"}` and the payload has exactly four claims:

```json
{"iss": "user-management", "sub": "6adf24024810ac71427b7f42", "exp": 1793012242, "iat": 1793008642}
```

| Claim | Meaning |
| --- | --- |
| `sub` | The user's ID |
| `iss` | The value of `JWT_ISSUER` |
| `iat` | Issued at (Unix seconds) |
| `exp` | Expires at: `iat` + `JWT_TTL` (1 hour by default) |

The email is deliberately not in the token, so it can't go stale after a `PATCH`.

### How tokens are validated

A token is accepted only if all of these hold:

- it is signed with HS256 and the configured secret (any other algorithm, including `none`, is rejected)
- `sub`, `iss`, `iat` and `exp` are all present, and `sub` is not empty
- `iss` equals `JWT_ISSUER`
- `exp` is in the future and `iat` is not

### Generating a secret

```sh
openssl rand -base64 32   # 44 characters, 32 random bytes
```

Shorter secrets are rejected at startup. Keep the secret out of the repository; `.env` files are git-ignored.

## Sample requests and responses

Recorded from the running server against MongoDB 7. `Date` and `Content-Length` headers are omitted, and `$TOKEN` is a token from the login example.

### Register

```sh
curl -i -X POST localhost:8080/auth/register -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"Alice@Example.com","password":"s3cret-pass"}'
```

```http
HTTP/1.1 201 Created
Content-Type: application/json

{"id":"6adf24024810ac71427b7f42","name":"Alice","email":"alice@example.com","createdAt":"2026-10-26T09:57:22.561Z"}
```

The email was stored lowercased. Registering the same email again:

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":"email already exists"}
```

Invalid input (`"email":"not-an-email"`):

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed: email must be a valid email address"}
```

An unknown field such as `"role":"admin"`:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"unknown field \"role\""}
```

### Log in

```sh
curl -i -X POST localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"s3cret-pass"}'
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJ1c2VyLW1hbmFnZW1lbnQiLCJzdWIiOiI2YWRmMjQwMjQ4MTBhYzcxNDI3YjdmNDIiLCJleHAiOjE3OTMwMTIyNDIsImlhdCI6MTc5MzAwODY0Mn0.AenQ20YX6Y8YtJAstOHkmNS3ThsuxfFbvVT-gRqCnRc"}
```

A wrong password, or an email that isn't registered, gets the same response:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":"invalid email or password"}
```

### List users

```sh
curl -i localhost:8080/users -H "Authorization: Bearer $TOKEN"
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

[{"id":"6adf24024810ac71427b7f42","name":"Alice","email":"alice@example.com","createdAt":"2026-10-26T09:57:22.561Z"},{"id":"6adf24024810ac71427b7f43","name":"Bob","email":"bob@example.com","createdAt":"2026-10-26T09:57:22.624Z"}]
```

With no users the response is `[]`. Without a token:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json
Www-Authenticate: Bearer

{"error":"missing bearer token"}
```

With an invalid or expired token:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json
Www-Authenticate: Bearer

{"error":"invalid or expired token"}
```

### Get a user

```sh
curl -i localhost:8080/users/6adf24024810ac71427b7f42 -H "Authorization: Bearer $TOKEN"
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"6adf24024810ac71427b7f42","name":"Alice","email":"alice@example.com","createdAt":"2026-10-26T09:57:22.561Z"}
```

A malformed ID (`/users/not-an-id`) and a well-formed ID that doesn't exist (`/users/000000000000000000000001`):

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid user id"}
```

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":"user not found"}
```

### Update a user

```sh
curl -i -X PATCH localhost:8080/users/6adf24024810ac71427b7f42 -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"Alice Smith"}'
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"6adf24024810ac71427b7f42","name":"Alice Smith","email":"alice@example.com","createdAt":"2026-10-26T09:57:22.561Z"}
```

Both fields at once (`{"name":"Alice S.","email":"alice.s@example.com"}`):

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"6adf24024810ac71427b7f42","name":"Alice S.","email":"alice.s@example.com","createdAt":"2026-10-26T09:57:22.561Z"}
```

An empty body `{}`, an attempt to change the password, and an email that belongs to someone else:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed: at least one of name or email is required"}
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"unknown field \"password\""}
```

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":"email already exists"}
```

### Delete a user

```sh
curl -i -X DELETE localhost:8080/users/6adf24024810ac71427b7f43 -H "Authorization: Bearer $TOKEN"
```

```http
HTTP/1.1 204 No Content
```

Deleting it again:

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":"user not found"}
```

The deleted user's email is free to register again.

### Unknown routes and wrong methods

`GET /nope` and `GET /auth/login`:

```http
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":"not found"}
```

```http
HTTP/1.1 405 Method Not Allowed
Allow: POST
Content-Type: application/json

{"error":"method not allowed"}
```

## Testing

### Running the tests

From `user-management/`:

```sh
go test ./...                 # all unit tests; no MongoDB or Docker needed
go test -race ./...           # with the race detector
go test -cover ./...          # with coverage per package
GOTOOLCHAIN=go1.26.0 go test -race ./...   # on the minimum supported Go version
```

### What the unit tests cover

All tests use the standard `testing` package. MongoDB is replaced by hand-written in-memory fakes in `internal/fake` that implement the same port interfaces as the real adapter and return the same domain errors (duplicate email, malformed ID, missing user). No mocking library is used.

| Package | Covered | Coverage |
| --- | --- | --- |
| `internal/service` | Registration, login, get, list, update, delete; validation rules and boundaries; email normalization; identical errors for wrong password and unknown email; error pass-through | 100% |
| `internal/adapter/http` | Every route and status code through `httptest`, the JWT middleware, request logging, JSON body limits, JSON 404/405, no password in any response, generic 500s | 97% |
| `internal/adapter/auth` | JWT issue and validation (expired, tampered, wrong secret, wrong algorithm including `none`, wrong issuer, each claim missing); bcrypt, including the 72-byte limit | 97% |
| `internal/worker` | The user-count loop, driven tick by tick through a channel: no sleeps, no real ticker | 100% |
| `cmd/api` | Configuration loading, the HTTP server timeouts, and graceful shutdown over a real TCP listener (an in-flight request finishes; on a shutdown timeout the stuck connection is closed; server error) | partial: the MongoDB wiring in `run` is not unit-tested |
| `internal/adapter/mongo` | ID parsing, driver error mapping and document conversion only | ~11% |

### Testing boundary

`go test` deliberately does not talk to MongoDB. The brief asks for unit tests with MongoDB mocked, and the services and handlers are fully covered through the fakes. The MongoDB adapter itself is therefore covered only for the parts that need no database, which is why its coverage is low. This is a scoping decision for this submission, not an oversight; repeatable integration tests are listed under [Known limitations](#known-limitations).

### Manual MongoDB verification

The adapter and the running application were checked against `mongo:7` in Docker on 2026-10-26:

- **Adapter, during development and again after upgrading the driver to v2.9.2:** the unique email index is created and creating it again is harmless; a created user reads back unchanged; a duplicate email is rejected; a malformed ID and a well-formed missing ID give different errors; users list oldest first; count, partial update (other fields untouched), update to a taken email, delete, delete twice, and registering a deleted user's email again.
- **End to end, against the running binary:** every endpoint with `curl`, including every status code in the samples above; deleting a user and registering the same email again (`201`); one request log line per request with no password or hash in the logs; the user count logged every 10 seconds, rising after registrations; and `SIGINT` during a slow upload: the upload still got its response, then the server, worker and MongoDB connection shut down in that order and the process exited with code 0.

## Architecture

The code follows a hexagonal (ports and adapters) layout. Business logic doesn't depend on HTTP, MongoDB, bcrypt or JWT; those are adapters plugged in at startup.

```
user-management/
├── cmd/api/              # main: configuration, wiring, HTTP server
└── internal/
    ├── domain/           # User entity and domain errors; imports nothing from this module
    ├── port/             # interfaces: UserRepository, PasswordHasher, TokenIssuer, TokenValidator
    ├── service/          # AuthService (register, login) and UserService (get, list, update, delete)
    ├── adapter/
    │   ├── http/         # handlers, middleware (auth, logging), JSON helpers, router
    │   ├── mongo/        # UserRepository on MongoDB
    │   └── auth/         # bcrypt hasher, HS256 JWT issuer and validator
    ├── worker/           # background user-count goroutine
    └── fake/             # in-memory fakes of the ports, for tests
```

- `domain` has no dependencies; `service` depends only on `domain` and `port`.
- Adapters implement ports (`adapter/mongo`, `adapter/auth`) or drive the services (`adapter/http`).
- Only `adapter/mongo` imports the MongoDB driver, and only `adapter/mongo` knows that IDs are ObjectIDs.
- `cmd/api` is the only place that knows about every piece.

## Assumptions and design decisions

The brief leaves several points open. These are the choices made, with the decision number from [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md).

- **Registration is the only way to create a user (D2, D14).** The brief lists both "user registration" and "create a new user". A protected `POST /users` had no clear use case next to public registration, so `POST /auth/register` covers both, and `AuthService.Register` is the only code that creates users.
- **Authorization is authentication only (D3).** The brief asks to protect endpoints with JWT; it doesn't define roles or ownership. Any logged-in user can read, update and delete any user. See [Known limitations](#known-limitations).
- **Standard library HTTP (D4).** Go 1.22+ `ServeMux` supports method and path patterns, which is enough for six routes and keeps handlers free of a framework.
- **Hand-written fakes instead of a mocking library (D5).** They behave like the real repository where it matters and keep the tests readable.
- **Tokens (D6, D17).** Access tokens only, 1-hour default lifetime, claims `sub`, `iss`, `iat` and `exp`, all required. No refresh tokens.
- **Partial update with `PATCH` (D7).** Matches "update a user's name or email": send either or both. The password can't be changed through the API.
- **List returns everyone (D8).** The brief says "list all users", so there is no pagination.
- **Hard delete (D9).** The document is removed, so the email can be registered again.
- **IDs (D15).** MongoDB ObjectIDs as 24-character hex strings. A malformed ID is a client error (`400`); a well-formed ID that doesn't exist is `404`.
- **Validation (D11, D21).** Lengths and email format as in [Request rules](#request-rules); unknown JSON fields are rejected so typos and attempts to set other fields fail loudly. Emails are lowercased as a whole, which treats the local part as case-insensitive like most providers do.
- **Hardening (D17).** 32-byte minimum secret, server timeouts, 1 MiB body limit, trailing JSON rejected.
- **No account enumeration.** Login answers a wrong password and an unknown email identically, and compares against a dummy bcrypt hash for unknown emails so the response time doesn't differ either.
- **Logging.** Request logs carry the status code as well as method, path and duration. The path is logged without the query string. Internal errors are logged in full but returned to the client as a generic `500`.
- **JSON field names are camelCase (D20).**
- **Go 1.26 minimum (D22).** The oldest Go release that still gets security fixes; dependencies are on their latest versions.

## Known limitations

- **Any authenticated user can update or delete any other user (D3).** There is no ownership check and no admin role. Possible future work: allow changes only to one's own account, or add roles.
- **Tokens can't be revoked (D6).** Tokens are stateless and there is no refresh token or deny list. In particular, **a token belonging to a deleted user stays valid until it expires** (at most `JWT_TTL`, 1 hour by default), and can still call the `/users` endpoints in that time.
- **No pagination (D8).** `GET /users` returns every user in one response.
- **No automated MongoDB integration tests (D5).** The adapter was verified manually (see [Manual MongoDB verification](#manual-mongodb-verification)); repeatable integration tests would be the next step.

## Bonus features

| Bonus | Status |
| --- | --- |
| Abstraction with Go interfaces | Done: every external dependency is behind a port in `internal/port` |
| Input validation | Done: see [Request rules](#request-rules) |
| Hexagonal architecture | Done: see [Architecture](#architecture) |
| Graceful shutdown with `context.Context` | Done: `SIGINT`/`SIGTERM` via `signal.NotifyContext`; see [Quick start](#3-run) |
| Docker and `docker-compose` | Planned |
| gRPC (`CreateUser`, `GetUser`) | Stretch goal, not started |
