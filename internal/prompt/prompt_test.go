package prompt_test

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/prompt"
)

func answers(given ...string) (func(string) (string, error), *[]string) {
	var asked []string
	return func(label string) (string, error) {
		asked = append(asked, label)
		if len(given) == 0 {
			return "", errors.New("no more input")
		}
		a := given[0]
		given = given[1:]
		return a, nil
	}, &asked
}

func TestConfirm_Match(t *testing.T) {
	read, asked := answers("a brand new password", "a brand new password")
	got, err := prompt.Confirm(read, "New password: ", "Repeat password: ")
	require.NoError(t, err)
	assert.Equal(t, "a brand new password", got)
	assert.Equal(t, []string{"New password: ", "Repeat password: "}, *asked)
}

func TestConfirm_Mismatch(t *testing.T) {
	read, _ := answers("a brand new password", "another password")
	_, err := prompt.Confirm(read, "New password: ", "Repeat password: ")
	require.ErrorIs(t, err, prompt.ErrMismatch)
	assert.NotContains(t, err.Error(), "a brand new password")
}

func TestLine(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        bool
	}{
		{name: "newline", in: "secret\n", want: "secret"},
		{name: "crlf", in: "secret\r\n", want: "secret"},
		{name: "last line without newline", in: "secret", want: "secret"},
		{name: "empty input", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prompt.Line(bufio.NewReader(strings.NewReader(tt.in)))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
