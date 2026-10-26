// Package grpc is the gRPC adapter (C5): CreateUser and GetUser over the same
// services as the REST API. The generated code is in the userpb subpackage;
// regenerate it from proto/user.proto as described in the README.
package grpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/grpc/userpb"
	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

// NewServer returns a gRPC server with the user service registered, request
// logging, bearer-token auth for everything except CreateUser (D23), and
// server reflection so grpcurl works without the .proto file.
func NewServer(auth *service.AuthService, users *service.UserService, tokens port.TokenValidator, logger *slog.Logger) *grpc.Server {
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		loggingInterceptor(logger),
		authInterceptor(tokens, publicMethods),
	))
	userpb.RegisterUserServiceServer(srv, &userServer{auth: auth, users: users, logger: logger})
	reflection.Register(srv)
	return srv
}

// publicMethods don't need a token: CreateUser mirrors POST /auth/register.
var publicMethods = map[string]bool{
	userpb.UserService_CreateUser_FullMethodName: true,
}

type userServer struct {
	userpb.UnimplementedUserServiceServer
	auth   *service.AuthService
	users  *service.UserService
	logger *slog.Logger
}

// CreateUser registers a user. Like POST /auth/register, it goes through
// AuthService.Register, the only place that creates users (D14).
func (s *userServer) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.CreateUserResponse, error) {
	user, err := s.auth.Register(ctx, service.RegisterInput{
		Name:     req.GetName(),
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &userpb.CreateUserResponse{User: toProto(user)}, nil
}

// GetUser returns a user by ID, like GET /users/{id}.
func (s *userServer) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	user, err := s.users.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &userpb.GetUserResponse{User: toProto(user)}, nil
}

// toProto maps a domain user to its message. There is no password field.
func toProto(u domain.User) *userpb.User {
	return &userpb.User{
		Id:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
}

// toStatus maps a service error to a gRPC status, the counterpart of the HTTP
// adapter's writeServiceError. Unknown errors are logged and answered with a
// generic Internal, so internal details never reach the client.
func (s *userServer) toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrInvalidID):
		return status.Error(codes.InvalidArgument, domain.ErrInvalidID.Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrEmailExists):
		return status.Error(codes.AlreadyExists, domain.ErrEmailExists.Error())
	default:
		s.logger.ErrorContext(ctx, "grpc request failed", "error", err)
		return status.Error(codes.Internal, "internal server error")
	}
}
