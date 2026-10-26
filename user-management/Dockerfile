# Build stage: compile a static binary.
FROM golang:1.27 AS build
WORKDIR /src

# Download modules first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Runtime stage: no shell or package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
# Exec form: the binary is PID 1 and receives SIGTERM from `docker stop`
# directly, which starts the graceful shutdown (C4).
ENTRYPOINT ["/api"]
