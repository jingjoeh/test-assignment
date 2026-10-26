package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/jingjoeh/test-assignment/user-management/internal/adapter/grpc/userpb"
	"github.com/jingjoeh/test-assignment/user-management/internal/fake"
	"github.com/jingjoeh/test-assignment/user-management/internal/service"
)

// anyToken is accepted by fake.Tokens.
const anyToken = "token-for:000000000000000000000001"

type testServer struct {
	client userpb.UserServiceClient
	conn   *grpc.ClientConn
	repo   *fake.UserRepository
	logs   *bytes.Buffer
}

// newTestServer runs the real gRPC server, wired to in-memory fakes, over an
// in-memory bufconn connection.
func newTestServer(t *testing.T) testServer {
	t.Helper()
	repo := fake.NewUserRepository()
	tokens := &fake.Tokens{}
	auth, err := service.NewAuthService(repo, &fake.Hasher{}, tokens)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	logs := &bytes.Buffer{}
	srv := NewServer(auth, service.NewUserService(repo), tokens, slog.New(slog.NewJSONHandler(logs, nil)))

	ln := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return testServer{client: userpb.NewUserServiceClient(conn), conn: conn, repo: repo, logs: logs}
}

func withAuth(value string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", value)
}

func (s testServer) createAlice(t *testing.T) *userpb.User {
	t.Helper()
	resp, err := s.client.CreateUser(context.Background(), &userpb.CreateUserRequest{
		Name: "Alice", Email: "Alice@Example.com", Password: "s3cret-pass",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return resp.GetUser()
}

func assertStatus(t *testing.T, err error, want codes.Code, wantMsg string) {
	t.Helper()
	st, _ := status.FromError(err)
	if st.Code() != want || !strings.Contains(st.Message(), wantMsg) {
		t.Errorf("status = %v %q, want %v containing %q", st.Code(), st.Message(), want, wantMsg)
	}
}

func TestCreateUserIsPublic(t *testing.T) {
	s := newTestServer(t)

	u := s.createAlice(t) // no authorization metadata

	if u.GetId() == "" || u.GetName() != "Alice" || u.GetEmail() != "alice@example.com" {
		t.Errorf("user = %v, want alice with a normalized email", u)
	}
	if !u.GetCreatedAt().IsValid() || u.GetCreatedAt().AsTime().IsZero() {
		t.Errorf("created_at = %v, want a set timestamp", u.GetCreatedAt())
	}
	stored, err := s.repo.GetByID(context.Background(), u.GetId())
	if err != nil || stored.PasswordHash != "hashed:s3cret-pass" {
		t.Errorf("stored user = %+v, %v; want the hashed password, created through AuthService", stored, err)
	}
}

func TestCreateUserErrors(t *testing.T) {
	s := newTestServer(t)
	s.createAlice(t)

	_, err := s.client.CreateUser(context.Background(), &userpb.CreateUserRequest{Name: "", Email: "b@example.com", Password: "s3cret-pass"})
	assertStatus(t, err, codes.InvalidArgument, "name is required")

	_, err = s.client.CreateUser(context.Background(), &userpb.CreateUserRequest{Name: "B", Email: "nope", Password: "s3cret-pass"})
	assertStatus(t, err, codes.InvalidArgument, "valid email")

	_, err = s.client.CreateUser(context.Background(), &userpb.CreateUserRequest{Name: "B", Email: "b@example.com", Password: "short"})
	assertStatus(t, err, codes.InvalidArgument, "at least 8 characters")

	_, err = s.client.CreateUser(context.Background(), &userpb.CreateUserRequest{Name: "A2", Email: "ALICE@example.com", Password: "s3cret-pass"})
	assertStatus(t, err, codes.AlreadyExists, "email already exists")
}

func TestGetUser(t *testing.T) {
	s := newTestServer(t)
	alice := s.createAlice(t)

	resp, err := s.client.GetUser(withAuth("Bearer "+anyToken), &userpb.GetUserRequest{Id: alice.GetId()})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	got := resp.GetUser()
	if got.GetId() != alice.GetId() || got.GetName() != "Alice" || got.GetEmail() != "alice@example.com" ||
		!got.GetCreatedAt().AsTime().Equal(alice.GetCreatedAt().AsTime()) {
		t.Errorf("GetUser = %v, want %v", got, alice)
	}

	_, err = s.client.GetUser(withAuth("Bearer "+anyToken), &userpb.GetUserRequest{Id: "not-an-id"})
	assertStatus(t, err, codes.InvalidArgument, "invalid user id")

	_, err = s.client.GetUser(withAuth("Bearer "+anyToken), &userpb.GetUserRequest{Id: fake.NewID(42)})
	assertStatus(t, err, codes.NotFound, "user not found")
}

func TestGetUserRequiresToken(t *testing.T) {
	s := newTestServer(t)
	alice := s.createAlice(t)
	req := &userpb.GetUserRequest{Id: alice.GetId()}

	tests := []struct {
		name    string
		ctx     context.Context
		wantMsg string
	}{
		{"no metadata", context.Background(), "missing bearer token"},
		{"wrong scheme", withAuth("Basic dXNlcjpwYXNz"), "missing bearer token"},
		{"scheme only", withAuth("Bearer"), "missing bearer token"},
		{"extra field", withAuth("Bearer " + anyToken + " extra"), "missing bearer token"},
		{"two authorization values", metadata.AppendToOutgoingContext(withAuth("Bearer "+anyToken), "authorization", "Bearer "+anyToken), "missing bearer token"},
		{"invalid token", withAuth("Bearer not-a-real-token"), "invalid or expired token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.client.GetUser(tt.ctx, req)
			assertStatus(t, err, codes.Unauthenticated, tt.wantMsg)
		})
	}

	// Same accepted forms as the HTTP middleware.
	for _, value := range []string{"bearer " + anyToken, "  BEARER   " + anyToken + "  "} {
		if _, err := s.client.GetUser(withAuth(value), req); err != nil {
			t.Errorf("GetUser with %q = %v, want success", value, err)
		}
	}
}

func TestInternalErrorIsGeneric(t *testing.T) {
	s := newTestServer(t)
	alice := s.createAlice(t)
	s.repo.Err = errors.New("mongo: connection refused to 10.0.0.5")

	_, err := s.client.GetUser(withAuth("Bearer "+anyToken), &userpb.GetUserRequest{Id: alice.GetId()})
	assertStatus(t, err, codes.Internal, "internal server error")
	if strings.Contains(err.Error(), "10.0.0.5") {
		t.Errorf("error leaks the internal cause: %v", err)
	}
	if !strings.Contains(s.logs.String(), "10.0.0.5") {
		t.Errorf("internal error was not logged; logs = %s", s.logs)
	}
}

func TestLoggingOneEntryPerCall(t *testing.T) {
	s := newTestServer(t)
	alice := s.createAlice(t)
	_, _ = s.client.GetUser(context.Background(), &userpb.GetUserRequest{Id: alice.GetId()}) // Unauthenticated

	lines := strings.Split(strings.TrimSpace(s.logs.String()), "\n")
	var entries []string
	for _, l := range lines {
		if strings.Contains(l, `"msg":"grpc request"`) {
			entries = append(entries, l)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("got %d grpc request entries, want 2:\n%s", len(entries), s.logs)
	}
	for i, want := range []string{`"method":"/user.v1.UserService/CreateUser","code":"OK"`, `"method":"/user.v1.UserService/GetUser","code":"Unauthenticated"`} {
		if !strings.Contains(entries[i], want) || !strings.Contains(entries[i], `"duration_ms":`) {
			t.Errorf("entry %d = %s, want %s with a duration", i, entries[i], want)
		}
	}
}

func TestReflectionListsUserService(t *testing.T) {
	s := newTestServer(t)

	stream, err := reflectionpb.NewServerReflectionClient(s.conn).ServerReflectionInfo(context.Background())
	if err != nil {
		t.Fatalf("ServerReflectionInfo: %v", err)
	}
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	var names []string
	for _, svc := range resp.GetListServicesResponse().GetService() {
		names = append(names, svc.GetName())
	}
	if !strings.Contains(strings.Join(names, ","), "user.v1.UserService") {
		t.Errorf("reflection lists %v, want user.v1.UserService", names)
	}
}
