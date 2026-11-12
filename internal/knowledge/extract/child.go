package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"time"
)

// DefaultChildTimeout bounds one child extraction.
const DefaultChildTimeout = 60 * time.Second

// MaxChildOutput caps the bytes a child may write.
const MaxChildOutput = 2 << 20

// DefaultChildMemory is the child's memory limit when Child.MemoryLimit is unset.
const DefaultChildMemory int64 = 512 << 20

// MaxPDFPages is the largest page count read; a bigger PDF is refused before any page is parsed.
const MaxPDFPages = 1000

// MaxChildren is how many extract children one instance runs at once; more wait for a slot.
const MaxChildren = 2

const childWaitDelay = 2 * time.Second

//nolint:gochecknoglobals // one limit for the whole process, shared by every Child value.
var childSlots = make(chan struct{}, MaxChildren)

var (
	errChildTimeout  = errors.New("extract: child deadline")
	errChildOverflow = errors.New("extract: child output over the cap")
)

// PDFReader is the PDF library behind the child process.
type PDFReader interface {
	PageCount(data []byte) (int, error)
	Pages(data []byte) ([]Section, error)
}

// Child runs extraction in a separate process with a deadline, a memory limit and an output cap.
type Child struct {
	Path        string
	Args        []string
	Env         []string
	Timeout     time.Duration
	MaxOutput   int64
	MemoryLimit int64
}

type wireError struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

type wireResult struct {
	Sections []Section  `json:"sections,omitempty"`
	Error    *wireError `json:"error,omitempty"`
}

type capWriter struct {
	buf      bytes.Buffer
	max      int64
	overflow func()
}

func (w *capWriter) Write(p []byte) (int, error) {
	if int64(w.buf.Len()+len(p)) > w.max {
		w.overflow()
		return 0, errChildOverflow
	}
	return w.buf.Write(p)
}

// Extract runs `<Path> <Args...> <kind>` with data on stdin and reads the sections from its stdout.
func (c Child) Extract(ctx context.Context, kind Kind, data []byte) ([]Section, error) {
	select {
	case childSlots <- struct{}{}:
		defer func() { <-childSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultChildTimeout
	}
	maxOut := c.MaxOutput
	if maxOut <= 0 {
		maxOut = MaxChildOutput
	}
	memory := c.MemoryLimit
	if memory <= 0 {
		memory = DefaultChildMemory
	}
	run, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	run, stop := context.WithTimeoutCause(run, timeout, errChildTimeout)
	defer stop()

	args := append(append([]string{}, c.Args...), string(kind))
	cmd := exec.CommandContext(run, c.Path, args...) //nolint:gosec // the operator's own binary, never user input
	cmd.Env = append(append([]string{}, c.Env...), "GOMEMLIMIT="+strconv.FormatInt(memory, 10))
	cmd.Stdin = bytes.NewReader(data)
	out := &capWriter{max: maxOut, overflow: func() { cancel(errChildOverflow) }}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = childWaitDelay
	if err := cmd.Start(); err != nil {
		return nil, &UnreadableError{Reason: Crashed, Cause: err}
	}
	limitMemory(cmd.Process.Pid, memory)
	err := cmd.Wait()
	switch cause := context.Cause(run); {
	case errors.Is(cause, errChildOverflow):
		return nil, &UnreadableError{Reason: Overflow}
	case errors.Is(cause, errChildTimeout):
		return nil, &UnreadableError{Reason: Timeout}
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, &UnreadableError{Reason: Crashed, Cause: err}
	}
	return decodeResult(out.buf.Bytes())
}

func decodeResult(raw []byte) ([]Section, error) {
	var res wireResult
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		return nil, &UnreadableError{Reason: Crashed, Cause: err}
	}
	if res.Error == nil {
		return nonEmpty(res.Sections)
	}
	switch res.Error.Kind {
	case "rejected":
		return nil, &RejectedError{Reason: RejectReason(res.Error.Reason)}
	case "no_text":
		return nil, &NoTextError{}
	default:
		return nil, &UnreadableError{Reason: UnreadReason(res.Error.Reason)}
	}
}

// Serve is the child side: it reads one document from in and writes the result to out as JSON.
func Serve(ctx context.Context, kind Kind, in io.Reader, out io.Writer, pdf PDFReader) error {
	limitSelf()
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	sections, err := serveOne(ctx, kind, data, pdf)
	return json.NewEncoder(out).Encode(resultOf(sections, err))
}

func serveOne(ctx context.Context, kind Kind, data []byte, pdf PDFReader) ([]Section, error) {
	if kind != KindPDF {
		return Extract(ctx, kind, data)
	}
	if pdf == nil {
		return nil, &RejectedError{Reason: UnsupportedType}
	}
	return readPDF(pdf, data)
}

func readPDF(pdf PDFReader, data []byte) (sections []Section, err error) {
	defer func() {
		if p := recover(); p != nil {
			sections, err = nil, &UnreadableError{Reason: Corrupt}
		}
	}()
	pages, err := pdf.PageCount(data)
	if err != nil {
		return nil, &UnreadableError{Reason: Corrupt, Cause: err}
	}
	if pages > MaxPDFPages {
		return nil, &RejectedError{Reason: TooManyPages}
	}
	read, err := pdf.Pages(data)
	if err != nil {
		return nil, &UnreadableError{Reason: Corrupt, Cause: err}
	}
	return nonEmpty(read)
}

func resultOf(sections []Section, err error) wireResult {
	if err == nil {
		return wireResult{Sections: sections}
	}
	if r, ok := AsRejectedError(err); ok {
		return wireResult{Error: &wireError{Kind: "rejected", Reason: string(r.Reason)}}
	}
	if IsNoTextError(err) {
		return wireResult{Error: &wireError{Kind: "no_text"}}
	}
	reason := Corrupt
	if u, ok := errors.AsType[*UnreadableError](err); ok {
		reason = u.Reason
	}
	return wireResult{Error: &wireError{Kind: "unreadable", Reason: string(reason)}}
}
