// Package render dispatches CLI output to text tables, JSON, or NDJSON.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// Format names one of the canonical CLI output modes.
type Format string

const (
	FormatText   Format = "text"
	FormatJSON   Format = "json"
	FormatNDJSON Format = "ndjson"
)

// EnvVar is read when --output is unset.
const EnvVar = "OAGENT_OUTPUT"

// Detect picks the output format: --output flag > OAGENT_OUTPUT env > TTY heuristic.
func Detect(cmd *cobra.Command) Format {
	if raw := flagValue(cmd, "output"); raw != "" {
		return normalize(raw)
	}
	if raw := os.Getenv(EnvVar); raw != "" {
		return normalize(raw)
	}
	if isTTY() {
		return FormatText
	}
	return FormatJSON
}

func normalize(s string) Format {
	switch Format(strings.ToLower(strings.TrimSpace(s))) {
	case FormatJSON:
		return FormatJSON
	case FormatNDJSON:
		return FormatNDJSON
	default:
		return FormatText
	}
}

func flagValue(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if f := cmd.Flag(name); f != nil && f.Changed {
		return f.Value.String()
	}
	if root := cmd.Root(); root != nil {
		if f := root.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return f.Value.String()
		}
	}
	return ""
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// JSON writes v indented as one JSON document.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// NDJSON writes each element of v as one JSON line; a non-slice is written as a single line.
func NDJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return enc.Encode(v)
	}
	for i := 0; i < rv.Len(); i++ {
		if err := enc.Encode(rv.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}

// Table writes rows under headers with aligned tab-separated columns.
func Table(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(headers) > 0 {
		if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// Error renders err to stderr: structured JSON/NDJSON, else a plain line.
func Error(cmd *cobra.Command, err error) {
	switch Detect(cmd) {
	case FormatJSON:
		_ = JSON(cmd.ErrOrStderr(), map[string]string{"error": err.Error()})
	case FormatNDJSON:
		_ = NDJSON(cmd.ErrOrStderr(), []map[string]string{{"error": err.Error()}})
	default:
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
	}
}
