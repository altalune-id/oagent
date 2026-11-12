package extract_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge/extract"
)

const childMode = "OAGENT_EXTRACT_TEST_CHILD"

type fakePDF struct {
	pages      int
	countErr   error
	pagesCalls *int
}

func (f fakePDF) PageCount([]byte) (int, error) { return f.pages, f.countErr }

func (f fakePDF) Pages(data []byte) ([]extract.Section, error) {
	if f.pagesCalls != nil {
		*f.pagesCalls++
	}
	out := []extract.Section{}
	for i, text := range strings.Split(string(data), "|") {
		if strings.TrimSpace(text) != "" {
			out = append(out, extract.Section{Page: i + 1, Text: text})
		}
	}
	return out, nil
}

func TestMain(m *testing.M) {
	mode := os.Getenv(childMode)
	if mode == "" {
		os.Exit(m.Run())
	}
	kind := extract.Kind(os.Args[len(os.Args)-1])
	switch mode {
	case "serve":
		if err := extract.Serve(context.Background(), kind, os.Stdin, os.Stdout, fakePDF{pages: 3}); err != nil {
			os.Exit(1)
		}
	case "serve-pdf":
		if err := extract.Serve(context.Background(), kind, os.Stdin, os.Stdout, extract.PDF{}); err != nil {
			os.Exit(1)
		}
	case "env":
		_, _ = os.Stdout.WriteString(`{"sections":[{"text":"GOMEMLIMIT=` + os.Getenv("GOMEMLIMIT") + ` HOME=` + os.Getenv("HOME") + `"}]}`)
	case "loop":
		for {
			time.Sleep(time.Millisecond)
		}
	case "spew":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), 3<<20))
	case "crash":
		panic("pdf parser blew up")
	case "garbage":
		_, _ = os.Stdout.WriteString(`{"sections":[],"surprise":1}`)
	default:
		platformChild(mode)
	}
	os.Exit(0)
}

func child(mode string, timeout time.Duration) extract.Child {
	return extract.Child{
		Path:        os.Args[0],
		Args:        []string{"-test.run=^$"},
		Env:         []string{childMode + "=" + mode, "GORACE=atexit_sleep_ms=0"},
		Timeout:     timeout,
		MaxOutput:   extract.MaxChildOutput,
		MemoryLimit: 256 << 20,
	}
}

func unread(t *testing.T, err error) extract.UnreadReason {
	t.Helper()
	var u *extract.UnreadableError
	require.True(t, errors.As(err, &u), "want an UnreadableError, got %v", err)
	return u.Reason
}

func TestChild_ExtractsPagesInAChildProcess(t *testing.T) {
	sections, err := child("serve", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("one|two|three"))
	require.NoError(t, err)
	assert.Equal(t, []extract.Section{{Page: 1, Text: "one"}, {Page: 2, Text: "two"}, {Page: 3, Text: "three"}}, sections)
}

func TestChild_NoTextAndRejectionCrossTheBoundary(t *testing.T) {
	_, err := child("serve", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("| |"))
	assert.True(t, extract.IsNoTextError(err), "got %v", err)
	_, err = child("serve", 10*time.Second).Extract(context.Background(), extract.KindText, []byte("\xe9t\xe9"))
	assert.Equal(t, extract.InvalidUTF8, reason(t, err))
}

func TestChild_LoopKilledAtDeadline(t *testing.T) {
	start := time.Now()
	_, err := child("loop", 300*time.Millisecond).Extract(context.Background(), extract.KindPDF, []byte("x"))
	assert.Equal(t, extract.Timeout, unread(t, err))
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestExtract_OutputCapped(t *testing.T) {
	_, err := child("spew", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("x"))
	assert.Equal(t, extract.Overflow, unread(t, err))
}

func TestChild_CrashIsUnreadable(t *testing.T) {
	_, err := child("crash", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("x"))
	assert.Equal(t, extract.Crashed, unread(t, err))
}

func TestChild_UnknownFieldsInTheResultAreRefused(t *testing.T) {
	_, err := child("garbage", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("x"))
	assert.Equal(t, extract.Crashed, unread(t, err))
}

func TestChild_GetsOnlyTheMemoryLimitAndTheGivenEnv(t *testing.T) {
	t.Setenv("HOME", "/should/not/leak")
	sections, err := child("env", 10*time.Second).Extract(context.Background(), extract.KindPDF, []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "GOMEMLIMIT=268435456 HOME=", sections[0].Text)
}

func TestChild_CallerCancelStopsIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_, err := child("loop", 10*time.Second).Extract(ctx, extract.KindPDF, []byte("x"))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestServe_PageCapCheckedFirst(t *testing.T) {
	calls := 0
	var out bytes.Buffer
	require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, strings.NewReader("a|b"), &out, fakePDF{pages: extract.MaxPDFPages + 1, pagesCalls: &calls}))
	assert.Equal(t, 0, calls, "pages are never read past the cap")
	assert.JSONEq(t, `{"error":{"kind":"rejected","reason":"too_many_pages"}}`, out.String())
}

func TestServe_NoPDFReaderIsUnsupported(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, strings.NewReader("a"), &out, nil))
	assert.JSONEq(t, `{"error":{"kind":"rejected","reason":"unsupported_type"}}`, out.String())
}

func TestServe_PageCountFailureIsUnreadable(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, strings.NewReader("a"), &out, fakePDF{countErr: errors.New("bad xref")}))
	assert.JSONEq(t, `{"error":{"kind":"unreadable","reason":"corrupt"}}`, out.String())
}

type panickyPDF struct{ inPages bool }

func (p panickyPDF) PageCount([]byte) (int, error) {
	if !p.inPages {
		panic("bad xref table")
	}
	return 1, nil
}

func (panickyPDF) Pages([]byte) ([]extract.Section, error) { panic("bad content stream") }

func TestServe_PanickingReaderIsCorrupt(t *testing.T) {
	for _, r := range []panickyPDF{{}, {inPages: true}} {
		var out bytes.Buffer
		require.NoError(t, extract.Serve(context.Background(), extract.KindPDF, strings.NewReader("x"), &out, r))
		assert.JSONEq(t, `{"error":{"kind":"unreadable","reason":"corrupt"}}`, out.String())
	}
}

func TestChild_RealPDFAcrossTheBoundary(t *testing.T) {
	sections, err := child("serve-pdf", 20*time.Second).Extract(context.Background(), extract.KindPDF, fixture(t, "text_3pages.pdf"))
	require.NoError(t, err)
	require.Len(t, sections, 3)
	for i, s := range sections {
		assert.Equal(t, i+1, s.Page)
	}
	_, err = child("serve-pdf", 20*time.Second).Extract(context.Background(), extract.KindPDF, fixture(t, "scanned_no_text.pdf"))
	assert.True(t, extract.IsNoTextError(err), "got %v", err)
}

func TestExtract_LoopingPDFKilledAtDeadline(t *testing.T) {
	start := time.Now()
	_, err := child("serve-pdf", 2*time.Second).Extract(context.Background(), extract.KindPDF, fixture(t, "looping_xref.pdf"))
	assert.Equal(t, extract.Timeout, unread(t, err), "the real library loops on the self-pointing /Prev; the deadline kills it")
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestChild_ZeroMemoryLimitGetsTheDefault(t *testing.T) {
	c := child("env", 10*time.Second)
	c.MemoryLimit = 0
	sections, err := c.Extract(context.Background(), extract.KindPDF, []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "GOMEMLIMIT=536870912 HOME=", sections[0].Text)
	assert.Equal(t, int64(512<<20), extract.DefaultChildMemory)
}

// SECURITY: at most MaxChildren extract children run at once per instance, so their memory limits add up to a bounded total.
func TestChild_AtMostTwoAtOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := child("loop", 30*time.Second)
	var wg sync.WaitGroup
	for range 2 * extract.MaxChildren {
		wg.Go(func() { _, _ = c.Extract(ctx, extract.KindPDF, []byte("x")) })
	}
	require.Eventually(t, func() bool { return extract.ChildrenRunning() == extract.MaxChildren }, 10*time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, extract.MaxChildren, extract.ChildrenRunning(), "the others wait for a slot")
	cancel()
	wg.Wait()
	assert.Zero(t, extract.ChildrenRunning(), "a cancelled wait takes no slot and every slot is given back")
}
