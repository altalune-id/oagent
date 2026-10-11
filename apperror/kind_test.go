package apperror

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryCodeHasAKind(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "codes.go", nil, 0)
	require.NoError(t, err)

	var codes []Code
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, v := range spec.Values {
			lit, ok := v.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			codes = append(codes, Code(s))
		}
		return true
	})
	require.NotEmpty(t, codes)

	for _, c := range codes {
		_, ok := kinds[c]
		assert.True(t, ok, "code %s has no kind", c)
	}
}

func TestKindOf(t *testing.T) {
	cases := map[Code]Kind{
		CodeAgentNotFound:          KindNotFound,
		CodeAPIKeyPersonOnlyScope:  KindInvalid,
		CodeForbidden:              KindForbidden,
		CodeUnauthenticated:        KindUnauthenticated,
		CodeOrgCreationDisabled:    KindFailedPrecondition,
		CodeInstallAlreadyDone:     KindAlreadyExists,
		CodeAuthInvalidCredentials: KindUnauthenticated,
		CodeUnexpected:             KindInternal,
		Code("BOGUS"):              KindInternal,
	}
	for code, want := range cases {
		t.Run(string(code), func(t *testing.T) {
			assert.Equal(t, want, KindOf(code))
		})
	}
}
