package canonjson_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/canonjson"
)

func TestOf(t *testing.T) {
	for in, want := range map[string]string{
		` { "b" : 2, "a" : [ 1, { "d": 4, "c": 3 } ] } `: `{"a":[1,{"c":3,"d":4}],"b":2}`,
		` false `: `false`,
		`"<a&b>"`: `"<a&b>"`,
		`25000.0`: `25000`,
		`1.5`:     `1.5`,
		`null`:    `null`,
		`"café"`:  `"café"`,
	} {
		got, err := canonjson.Of([]byte(in))
		require.NoError(t, err, in)
		assert.Equal(t, want, string(got), "the runtime's canonicalJson writes the same: %s", in)
	}
	for _, bad := range []string{``, `fals`, `{"a":}`, `1 2`} {
		_, err := canonjson.Of([]byte(bad))
		assert.Error(t, err, bad)
	}
}

type vectorFile struct {
	About      string   `json:"about"`
	NormalForm []string `json:"normalForm"`
	Vectors    []struct {
		Name      string `json:"name"`
		Input     string `json:"input"`
		Canonical string `json:"canonical"`
		ArgsHash  string `json:"argsHash"`
		Error     bool   `json:"error"`
	} `json:"vectors"`
}

func TestOf_SharedVectorsMatchTheRuntime(t *testing.T) {
	raw, err := os.ReadFile("../internal/kernel/runtime/testdata/canonical_vectors.json")
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var file vectorFile
	require.NoError(t, dec.Decode(&file))
	require.NotEmpty(t, file.Vectors)
	for _, v := range file.Vectors {
		got, err := canonjson.Of([]byte(v.Input))
		if v.Error {
			assert.Error(t, err, v.Name)
			continue
		}
		require.NoError(t, err, v.Name)
		assert.Equal(t, v.Canonical, string(got), v.Name)
		sum := sha256.Sum256(got)
		assert.Equal(t, v.ArgsHash, hex.EncodeToString(sum[:]), "argsHash: %s", v.Name)
	}
}
