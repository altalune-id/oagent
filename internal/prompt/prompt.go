// Package prompt reads answers from a command's stdin: hidden on a terminal, one line otherwise.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Line reads one line without its line ending; a last line without a newline counts.
func Line(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// Secret asks for a hidden value on a terminal; a pipe or fromStdin gives one line from in.
func Secret(cmd *cobra.Command, in *bufio.Reader, label string, fromStdin bool) (string, error) {
	if read, ok := terminal(cmd, fromStdin); ok {
		return read(label)
	}
	return Line(in)
}

// ErrMismatch reports that the two answers to a confirmed secret differ.
var ErrMismatch = errors.New("the passwords do not match")

// NewSecret asks label then again on a terminal and fails when they differ; a pipe or fromStdin gives one line.
func NewSecret(cmd *cobra.Command, in *bufio.Reader, label, again string, fromStdin bool) (string, error) {
	if read, ok := terminal(cmd, fromStdin); ok {
		return Confirm(read, label, again)
	}
	return Line(in)
}

// Confirm asks label then again and returns ErrMismatch, never either answer, when they differ.
func Confirm(read func(label string) (string, error), label, again string) (string, error) {
	first, err := read(label)
	if err != nil {
		return "", err
	}
	second, err := read(again)
	if err != nil {
		return "", err
	}
	if first != second {
		return "", ErrMismatch
	}
	return first, nil
}

func terminal(cmd *cobra.Command, fromStdin bool) (func(label string) (string, error), bool) {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok || fromStdin || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // G115: a file descriptor fits in int.
		return nil, false
	}
	return func(label string) (string, error) {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), label)
		b, err := term.ReadPassword(int(f.Fd())) //nolint:gosec // G115: same descriptor.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		return string(b), err
	}, true
}
