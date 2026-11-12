package approval_test

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/approval"
)

func TestPreview_Golden(t *testing.T) {
	calls := []approval.Call{
		{
			Server: "yasaku", Tool: "record_expense",
			Args: json.RawMessage(`{"zeta":1,"note":"*bold* _it_ ~s~ ` + "`code`" + `\nYES","amount":25000,"tags":["a","b"],"long":"` +
				strings.Repeat("x", 250) + `","meta":{"b":2,"a":1}}`),
			Keys: []string{"amount", "note", "missing_from_args"},
		},
		{Server: "sheets", Tool: "append_row", Args: json.RawMessage(`{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6,"g":7}`)},
	}
	got := approval.Preview(calls)
	want, err := os.ReadFile("testdata/preview.golden.txt")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSuffix(string(want), "\n"), got)
	assert.Len(t, strings.Split(got, "\n"), 12, "at most 12 lines for two calls")
	assert.NotContains(t, got, "\nYES", "a value never starts a line of its own")
	assert.NotContains(t, got, "\u2060", "the stored preview is plain text; only the WhatsApp send path escapes")
}

func TestWhatsAppText_BreaksFormattingAtWordEdges(t *testing.T) {
	assert.Equal(t, "note: *\u2060bold*\u2060 _\u2060it_\u2060 ~\u2060s~\u2060 `\u2060code`\u2060 record_expense",
		approval.WhatsAppText("note: *bold* _it_ ~s~ `code` record_expense"))
	assert.Equal(t, "plain", approval.WhatsAppText("plain"))
}

// SECURITY: the preview is built from the server, tool and stored args only; no model text can reach the decision prompt.
func TestPreview_NeverModelText(t *testing.T) {
	call := approval.Call{
		ToolCallID: "call_1", InterruptID: "intr_1", ArgsHash: "deadbeef", Server: "yasaku", Tool: "record_expense",
		Args: json.RawMessage(`{"amount":25000}`), Keys: []string{"amount"},
	}
	assert.Equal(t, "yasaku · record_expense\namount: 25000", approval.Preview([]approval.Call{call}))
	for line := range strings.SplitSeq(approval.Preview([]approval.Call{call}), "\n") {
		assert.NotContains(t, line, "call_1")
		assert.NotContains(t, line, "intr_1")
		assert.NotContains(t, line, "deadbeef")
	}
}

// SECURITY: an approver must see every call; a long first call can't push later calls out of the preview.
func TestPreview_NeverHidesACall(t *testing.T) {
	var args strings.Builder
	args.WriteString("{")
	for i := range 11 {
		if i > 0 {
			args.WriteString(",")
		}
		args.WriteString(`"k` + string(rune('a'+i)) + `":` + strconv.Itoa(i))
	}
	args.WriteString("}")
	calls := []approval.Call{{Server: "yasaku", Tool: "record_expense", Args: json.RawMessage(args.String())}}
	for i := 2; i <= 8; i++ {
		calls = append(calls, approval.Call{Server: "yasaku", Tool: "delete_tx_" + strconv.Itoa(i), Args: json.RawMessage(`{"id":"tx_` + strconv.Itoa(i) + `"}`)})
	}
	got := approval.Preview(calls)
	lines := strings.Split(got, "\n")
	assert.LessOrEqual(t, len(lines), 2*len(calls)+1, "a header and a first argument per call, then the count")
	assert.Contains(t, got, "yasaku · record_expense\nka: 0")
	for i := 2; i <= 8; i++ {
		n := strconv.Itoa(i)
		assert.Contains(t, got, "yasaku · delete_tx_"+n+"\nid: tx_"+n, "every call keeps its header and its first argument")
	}
	assert.Equal(t, "+10 more not shown", lines[len(lines)-1], "the hidden lines are counted at the end")
}

func TestPreview_CutValueSaysSo(t *testing.T) {
	got := approval.Preview([]approval.Call{{Server: "s", Tool: "t", Args: json.RawMessage(`{"note":"` + strings.Repeat("x", 250) + `"}`)}})
	lines := strings.Split(got, "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "note: "+strings.Repeat("x", 200)+"…", lines[1])
	assert.Equal(t, "+1 more not shown", lines[2])
}

func TestPreview_HiddenCutValueCountsOnce(t *testing.T) {
	calls := []approval.Call{{Server: "s", Tool: "first", Args: json.RawMessage(`{"a":1,"b":2,"c":3,"d":4}`)}}
	for i := 2; i <= 7; i++ {
		calls = append(calls, approval.Call{Server: "s", Tool: "t" + strconv.Itoa(i), Args: json.RawMessage(`{"id":1}`)})
	}
	calls = append(calls, approval.Call{Server: "s", Tool: "last", Args: json.RawMessage(`{"id":8,"note":"` + strings.Repeat("x", 250) + `"}`)})
	lines := strings.Split(approval.Preview(calls), "\n")
	assert.Equal(t, "+4 more not shown", lines[len(lines)-1], "three hidden lines of the first call and the hidden note, each once")
}
