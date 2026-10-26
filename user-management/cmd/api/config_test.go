package main

import (
	"strings"
	"testing"
	"time"
)

func validEnv() map[string]string {
	return map[string]string{
		"MONGO_URI":  "mongodb://localhost:27017",
		"MONGO_DB":   "users",
		"JWT_SECRET": strings.Repeat("s", 32),
		"JWT_ISSUER": "user-management",
	}
}

func getenvFrom(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestLoadConfigValid(t *testing.T) {
	cfg, err := LoadConfig(getenvFrom(validEnv()))
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %q, want default %q", cfg.Port, defaultPort)
	}
	if cfg.JWTTTL != time.Hour {
		t.Errorf("JWTTTL = %s, want default 1h", cfg.JWTTTL)
	}
	if len(cfg.JWTSecret) != 32 {
		t.Errorf("len(JWTSecret) = %d, want 32", len(cfg.JWTSecret))
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	env := validEnv()
	env["PORT"] = "9090"
	env["JWT_TTL"] = "15m"

	cfg, err := LoadConfig(getenvFrom(env))
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.JWTTTL != 15*time.Minute {
		t.Errorf("JWTTTL = %s, want 15m", cfg.JWTTTL)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(env map[string]string)
		wantErr string
	}{
		{"missing MONGO_URI", func(e map[string]string) { delete(e, "MONGO_URI") }, "MONGO_URI is required"},
		{"missing MONGO_DB", func(e map[string]string) { delete(e, "MONGO_DB") }, "MONGO_DB is required"},
		{"missing JWT_ISSUER", func(e map[string]string) { delete(e, "JWT_ISSUER") }, "JWT_ISSUER is required"},
		{"missing JWT_SECRET", func(e map[string]string) { delete(e, "JWT_SECRET") }, "JWT_SECRET must be at least 32 bytes"},
		{"31-byte JWT_SECRET", func(e map[string]string) { e["JWT_SECRET"] = strings.Repeat("s", 31) }, "JWT_SECRET must be at least 32 bytes, got 31"},
		{"invalid JWT_TTL", func(e map[string]string) { e["JWT_TTL"] = "soon" }, "JWT_TTL is not a valid duration"},
		{"zero JWT_TTL", func(e map[string]string) { e["JWT_TTL"] = "0s" }, "JWT_TTL must be positive"},
		{"negative JWT_TTL", func(e map[string]string) { e["JWT_TTL"] = "-1h" }, "JWT_TTL must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnv()
			tt.mutate(env)

			_, err := LoadConfig(getenvFrom(env))
			if err == nil {
				t.Fatal("LoadConfig: expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfigReportsAllProblems(t *testing.T) {
	_, err := LoadConfig(getenvFrom(map[string]string{}))
	if err == nil {
		t.Fatal("LoadConfig: expected an error, got nil")
	}
	for _, want := range []string{"MONGO_URI", "MONGO_DB", "JWT_ISSUER", "JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %s", err, want)
		}
	}
}
