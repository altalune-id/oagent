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

// ErrCancelled reports a yes/no question answered with anything but yes, including no answer at all.
var ErrCancelled = errors.New("cancelled")

// YesNo asks question on stderr and reads one line from stdin; only y or yes agree, anything else is ErrCancelled.
func YesNo(cmd *cobra.Command, question string) error {
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), question)
	answer, err := Line(bufio.NewReader(cmd.InOrStdin()))
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return ErrCancelled
}

// Terminal is a stdin that can read a line without echoing it; a terminal *os.File is one through term.
type Terminal interface {
	ReadPassword() ([]byte, error)
}

func terminal(cmd *cobra.Command, fromStdin bool) (func(label string) (string, error), bool) {
	if fromStdin {
		return nil, false
	}
	var read func() ([]byte, error)
	switch in := cmd.InOrStdin().(type) {
	case Terminal:
		read = in.ReadPassword
	case *os.File:
		fd := int(in.Fd()) //nolint:gosec // G115: a file descriptor fits in int.
		if !term.IsTerminal(fd) {
			return nil, false
		}
		read = func() ([]byte, error) { return term.ReadPassword(fd) }
	default:
		return nil, false
	}
	return func(label string) (string, error) {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), label)
		b, err := read()
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		return string(b), err
	}, true
}
