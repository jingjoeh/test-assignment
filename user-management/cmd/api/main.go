// Command api runs the User Management REST API.
package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Wiring (Mongo, services, HTTP server, worker) is added in B8.
	logger.Info("configuration loaded", "port", cfg.Port, "mongo_db", cfg.MongoDB, "jwt_issuer", cfg.JWTIssuer, "jwt_ttl", cfg.JWTTTL.String())
}
