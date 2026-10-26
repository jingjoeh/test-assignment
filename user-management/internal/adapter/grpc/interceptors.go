package grpc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// loggingInterceptor logs one entry per call with the method, status code and
// duration, the gRPC counterpart of the HTTP logging middleware (req. 5).
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logger.InfoContext(ctx, "grpc request",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
		)
		return resp, err
	}
}

// authInterceptor requires "authorization: Bearer <token>" metadata on every
// method not in public, with the same token rules as the HTTP middleware.
func authInterceptor(tokens port.TokenValidator, public map[string]bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return handler(ctx, req)
		}
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) != 1 {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		token, ok := bearerToken(values[0])
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		if _, err := tokens.Validate(token); err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return handler(ctx, req)
	}
}

// bearerToken extracts the token from an authorization value: exactly two
// whitespace-separated fields with a case-insensitive "Bearer" scheme. It
// matches the HTTP adapter's rule; adapters don't import each other, so the
// few lines are repeated here.
func bearerToken(value string) (string, bool) {
	fields := strings.Fields(value)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}
