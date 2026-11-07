package channel

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

func TestDeviceKey_OpensTheSealedKey(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	c := &Connection{ID: uuid.New(), OrgID: uuid.New()}
	c.DeviceKeySealed, err = seal(sl, c, slotDeviceKey, "owk_device")
	require.NoError(t, err)
	got, err := (&Service{sealer: sl}).deviceKey(t.Context(), c)
	require.NoError(t, err)
	assert.Equal(t, "owk_device", got)

	_, err = (&Service{sealer: sealer.Disabled()}).deviceKey(t.Context(), c)
	assert.True(t, IsSecretsUnavailableError(err), "the caller still sees the same error: %v", err)
}

// SECURITY: a second place that opens the device key would skip whatever the one resolution point does.
func TestDeviceKey_IsTheOnlyPlaceTheKeyIsOpened(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	openers := map[string]int{}
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
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "open" {
					return true
				}
				for _, arg := range call.Args {
					if a, ok := arg.(*ast.Ident); ok && a.Name == "slotDeviceKey" {
						openers[fn.Name.Name]++
					}
				}
				return true
			})
		}
	}
	assert.Equal(t, map[string]int{"deviceKey": 1}, openers)
}
