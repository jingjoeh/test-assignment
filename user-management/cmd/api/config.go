package main

import (
	"errors"
	"fmt"
	"time"
)

const (
	defaultPort       = "8080"
	defaultGRPCPort   = "9090"
	defaultJWTTTL     = time.Hour
	minJWTSecretBytes = 32 // 256 bits, the HS256 key size
)

// Config holds the settings read from environment variables.
type Config struct {
	Port      string
	GRPCPort  string
	MongoURI  string
	MongoDB   string
	JWTSecret []byte
	JWTTTL    time.Duration
	JWTIssuer string
}

// LoadConfig reads the configuration through getenv (os.Getenv in production).
// It reports every problem at once so a misconfigured deployment fails fast
// with a single, complete error.
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:      getenv("PORT"),
		GRPCPort:  getenv("GRPC_PORT"),
		MongoURI:  getenv("MONGO_URI"),
		MongoDB:   getenv("MONGO_DB"),
		JWTSecret: []byte(getenv("JWT_SECRET")),
		JWTTTL:    defaultJWTTTL,
		JWTIssuer: getenv("JWT_ISSUER"),
	}
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = defaultGRPCPort
	}

	var errs []error
	for _, req := range []struct{ name, value string }{
		{"MONGO_URI", cfg.MongoURI},
		{"MONGO_DB", cfg.MongoDB},
		{"JWT_ISSUER", cfg.JWTIssuer},
	} {
		if req.value == "" {
			errs = append(errs, fmt.Errorf("%s is required", req.name))
		}
	}

	if len(cfg.JWTSecret) < minJWTSecretBytes {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes, got %d", minJWTSecretBytes, len(cfg.JWTSecret)))
	}

	if raw := getenv("JWT_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("JWT_TTL is not a valid duration: %q", raw))
		case ttl <= 0:
			errs = append(errs, fmt.Errorf("JWT_TTL must be positive, got %s", ttl))
		default:
			cfg.JWTTTL = ttl
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}
