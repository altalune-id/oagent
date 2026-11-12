//go:build linux

package extract_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"altalune.id/oagent/internal/knowledge/extract"
)

const headroom = 256 << 20

func platformChild(mode string) {
	if mode != "rlimit" {
		return
	}
	_ = extract.Serve(context.Background(), extract.KindText, strings.NewReader("x"), io.Discard, nil)
	var r unix.Rlimit
	_ = unix.Getrlimit(unix.RLIMIT_DATA, &r)
	_, _ = os.Stdout.WriteString(`{"sections":[{"text":"` + strconv.FormatUint(r.Cur, 10) + `"}]}`)
}

func selfLimit(t *testing.T, gomemlimit string) uint64 {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$", "text")
	cmd.Env = []string{childMode + "=rlimit", "GORACE=atexit_sleep_ms=0"}
	if gomemlimit != "" {
		cmd.Env = append(cmd.Env, "GOMEMLIMIT="+gomemlimit)
	}
	out, err := cmd.Output()
	require.NoError(t, err)
	var res struct {
		Sections []struct {
			Text string `json:"text"`
		} `json:"sections"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	v, err := strconv.ParseUint(res.Sections[0].Text, 10, 64)
	require.NoError(t, err)
	return v
}

func TestLimitSelf_SetsDataLimitFromGOMEMLIMIT(t *testing.T) {
	assert.Equal(t, uint64(100<<20+headroom), selfLimit(t, strconv.Itoa(100<<20)))
}

func TestLimitSelf_LeavesTheLimitWhenUnsetOrZero(t *testing.T) {
	var parent unix.Rlimit
	require.NoError(t, unix.Getrlimit(unix.RLIMIT_DATA, &parent))
	assert.Equal(t, parent.Cur, selfLimit(t, ""))
	assert.Equal(t, parent.Cur, selfLimit(t, "0"))
}

func TestChild_DefaultMemoryLimitReachesTheChild(t *testing.T) {
	c := child("rlimit", 10*time.Second)
	c.MemoryLimit = 0
	sections, err := c.Extract(context.Background(), extract.KindText, []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatUint(uint64(extract.DefaultChildMemory)+headroom, 10), sections[0].Text)
}
