package authn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
)

const (
	procRead     = "/test.v1.TestService/Read"
	procBoth     = "/test.v1.TestService/Both"
	procEmpty    = "/test.v1.TestService/Empty"
	procUnmapped = "/test.v1.TestService/Unmapped"
)

type seen struct {
	called bool
	p      session.Principal
	sid    string
	err    error
}

func toConnect() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}
			ae, ok := apperror.AsAppError(err)
			if !ok {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			switch apperror.KindOf(ae.Code()) {
			case apperror.KindUnauthenticated:
				return nil, connect.NewError(connect.CodeUnauthenticated, ae)
			case apperror.KindForbidden:
				return nil, connect.NewError(connect.CodePermissionDenied, ae)
			default:
				return nil, connect.NewError(connect.CodeInternal, ae)
			}
		}
	}
}

func recordErr(s *seen) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			s.err = err
			return resp, err
		}
	}
}

func serve(t *testing.T, o InterceptorOptions, s *seen) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, proc := range []string{procRead, procBoth, procEmpty, procUnmapped} {
		mux.Handle(proc, connect.NewUnaryHandler(proc,
			func(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
				s.called = true
				s.p = session.PrincipalFrom(ctx)
				s.sid, _ = session.SIDFrom(ctx)
				return connect.NewResponse(&emptypb.Empty{}), nil
			},
			connect.WithInterceptors(toConnect(), recordErr(s), Interceptor(o)),
		))
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, proc, auth string) error {
	t.Helper()
	client := connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+proc)
	req := connect.NewRequest(&emptypb.Empty{})
	if auth != "" {
		req.Header().Set("Authorization", auth)
	}
	_, err := client.CallUnary(context.Background(), req)
	return err
}

func TestInterceptor(t *testing.T) {
	reader := session.Principal{Source: session.SourceAPIKey, Scopes: []string{"agent:read"}}
	writerOnly := session.Principal{Source: session.SourceAPIKey, Scopes: []string{"webhook:write"}}
	cases := []struct {
		name     string
		required bool
		auth     string
		proc     string
		authn    Authenticator
		want     connect.Code
		reached  bool
	}{
		{"nothing, optional", false, "", procRead, &fakeAuthn{p: reader}, 0, true},
		{"nothing, required", true, "", procRead, &fakeAuthn{p: reader}, connect.CodeUnauthenticated, false},
		{"malformed, optional", false, "Bearer nope", procRead, &fakeAuthn{p: reader}, connect.CodeUnauthenticated, false},
		{"malformed, required", true, "Bearer nope", procRead, &fakeAuthn{p: reader}, connect.CodeUnauthenticated, false},
		{"rejected, optional", false, "Bearer oagent_x", procRead, &fakeAuthn{err: errors.New("revoked")}, connect.CodeUnauthenticated, false},
		{"rejected, required", true, "Bearer oagent_x", procRead, &fakeAuthn{err: errors.New("revoked")}, connect.CodeUnauthenticated, false},
		{"unmapped, optional", false, "Bearer oagent_x", procUnmapped, &fakeAuthn{p: reader}, connect.CodePermissionDenied, false},
		{"unmapped, required", true, "Bearer oagent_x", procUnmapped, &fakeAuthn{p: reader}, connect.CodePermissionDenied, false},
		{"not allowed, optional", false, "Bearer oagent_x", procRead, &fakeAuthn{p: writerOnly}, connect.CodePermissionDenied, false},
		{"not allowed, required", true, "Bearer oagent_x", procRead, &fakeAuthn{p: writerOnly}, connect.CodePermissionDenied, false},
		{"allowed, optional", false, "Bearer oagent_x", procRead, &fakeAuthn{p: reader}, 0, true},
		{"allowed, required", true, "Bearer oagent_x", procRead, &fakeAuthn{p: reader}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &seen{}
			srv := serve(t, InterceptorOptions{
				Chain:        Chain{tc.authn},
				KeyPrefix:    testPrefix,
				CookieSecret: testSecret,
				Permissions:  PermissionTable{procRead: {"agent:read"}},
				Required:     tc.required,
			}, s)

			err := call(t, srv, tc.proc, tc.auth)
			if tc.want == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, tc.want, connect.CodeOf(err))
			}
			assert.Equal(t, tc.reached, s.called)
		})
	}
}

func TestInterceptor_PrincipalInContext(t *testing.T) {
	want := session.Principal{Source: session.SourceAPIKey, Name: "ci", Scopes: []string{"agent:read"}}
	s := &seen{}
	srv := serve(t, InterceptorOptions{
		Chain:       Chain{&fakeAuthn{p: want}},
		KeyPrefix:   testPrefix,
		Permissions: PermissionTable{procRead: {"agent:read"}},
	}, s)

	require.NoError(t, call(t, srv, procRead, "Bearer oagent_x"))
	assert.Equal(t, want, s.p)
}

func TestInterceptor_NoCredentialHasNoPrincipal(t *testing.T) {
	s := &seen{}
	srv := serve(t, InterceptorOptions{Chain: Chain{&fakeAuthn{}}, KeyPrefix: testPrefix}, s)

	require.NoError(t, call(t, srv, procRead, ""))
	assert.Empty(t, s.p.Source)
}

func TestInterceptor_AnyAuthenticated(t *testing.T) {
	s := &seen{}
	srv := serve(t, InterceptorOptions{
		Chain:       Chain{&fakeAuthn{p: session.Principal{Source: session.SourceAPIKey}}},
		KeyPrefix:   testPrefix,
		Permissions: PermissionTable{procRead: {authz.AnyAuthenticated}},
	}, s)

	require.NoError(t, call(t, srv, procRead, "Bearer oagent_x"))
	assert.True(t, s.called)
}

// SECURITY: a procedure that lists several permissions needs every one of them.
func TestInterceptor_AllOf(t *testing.T) {
	table := PermissionTable{procBoth: {"agent:read", "webhook:read"}, procEmpty: {}}
	cases := []struct {
		name    string
		proc    string
		scopes  []string
		denied  string
		reached bool
	}{
		{"holds both", procBoth, []string{"agent:read", "webhook:read"}, "", true},
		{"holds only the first", procBoth, []string{"agent:read"}, "webhook:read", false},
		{"holds only the second", procBoth, []string{"webhook:read"}, "agent:read", false},
		{"empty entry", procEmpty, []string{"*"}, "", false},
		{"unmapped", procUnmapped, []string{"*"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &seen{}
			p := session.Principal{Source: session.SourceLocal, Scopes: tc.scopes}
			srv := serve(t, InterceptorOptions{Chain: Chain{&fakeAuthn{p: p}}, KeyPrefix: testPrefix, Permissions: table}, s)

			err := call(t, srv, tc.proc, "Bearer oagent_x")
			assert.Equal(t, tc.reached, s.called)
			if tc.reached {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			forbidden, ok := errors.AsType[*authz.ForbiddenError](s.err)
			require.True(t, ok, "got %v", s.err)
			assert.Equal(t, tc.denied, forbidden.Permission)
		})
	}
}

func TestInterceptor_SessionIDInContext(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	s := &seen{}
	srv := serve(t, InterceptorOptions{
		Chain:        Chain{&fakeAuthn{p: session.Principal{Source: session.SourceLocal, Name: "ada"}}},
		KeyPrefix:    testPrefix,
		CookieSecret: secret,
		Permissions:  PermissionTable{procRead: {authz.AnyAuthenticated}},
	}, s)

	client := connect.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+procRead)
	req := connect.NewRequest(&emptypb.Empty{})
	req.Header().Set("Cookie", session.CookieName+"="+session.Sign(secret, "sid-1"))
	_, err := client.CallUnary(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "sid-1", s.sid)

	require.NoError(t, call(t, srv, procRead, "Bearer oagent_x"))
	assert.Empty(t, s.sid, "a key carries no session id")
}
