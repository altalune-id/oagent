package mcpserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/sealer"
)

func TestCredential_OpensWhatTheServerCarries(t *testing.T) {
	sl := testSealer(t)
	svc := NewService(nil, nil, nil).WithSealer(sl)
	for _, kind := range []AuthKind{AuthBearer, AuthHeader} {
		srv := &Server{ID: uuid.New(), OrgID: uuid.New(), AuthKind: kind}
		sealed, err := sealAuth(sl, srv.OrgID, srv.ID, "tok-"+string(kind))
		require.NoError(t, err)
		srv.AuthSealed = sealed
		got, err := svc.credential(t.Context(), srv)
		require.NoError(t, err)
		assert.Equal(t, "tok-"+string(kind), got)
	}
	got, err := svc.credential(t.Context(), &Server{ID: uuid.New(), OrgID: uuid.New(), AuthKind: AuthNone, AuthSealed: []byte{1}})
	require.NoError(t, err)
	assert.Empty(t, got, "a server without auth carries nothing")

	_, err = NewService(nil, nil, nil).credential(t.Context(), &Server{ID: uuid.New(), OrgID: uuid.New(), AuthKind: AuthBearer, AuthSealed: []byte{1}})
	assert.True(t, sealer.IsUnavailableError(err), "the caller still sees the sealer's error: %v", err)
}

// SECURITY: a second place that opens the credential would skip whatever the one resolution point does.
func TestCredential_IsTheOnlyPlaceAnAuthValueIsOpened(t *testing.T) {
	assert.Equal(t, map[string]int{"credential": 1}, callersOf(t, "openAuth"))
}

func callersOf(t *testing.T, name string) map[string]int {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	out := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		require.NoError(t, err)
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
						out[fn.Name.Name]++
					}
				}
				return true
			})
		}
	}
	return out
}
