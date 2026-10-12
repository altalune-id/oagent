// Package console is the development mail driver that prints each message instead of sending it.
package console

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"altalune.id/oagent/mailer"
)

var _ mailer.Mailer = (*Sender)(nil)

// Sender prints each Message to its writer.
type Sender struct {
	from string
	out  io.Writer
}

// New returns a Sender that fills an empty From with from and prints to w, or to os.Stdout when w is nil.
func New(from string, w io.Writer) *Sender {
	if w == nil {
		w = os.Stdout
	}
	return &Sender{from: from, out: w}
}

// Send validates m and prints its headers and both bodies.
func (s *Sender) Send(_ context.Context, m mailer.Message) error {
	m.From = cmp.Or(m.From, s.from)
	if err := m.Validate(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(s.out,
		"\n--- mailer.console %s ---\nFrom: %s\nTo: %s\nSubject: %s\n\n%s\n\n%s\n------------------------------\n",
		time.Now().UTC().Format(time.RFC3339), m.From, m.To, m.Subject, m.TextBody, m.HTMLBody)
	if err != nil {
		return fmt.Errorf("mailer: console: %w", err)
	}
	return nil
}
