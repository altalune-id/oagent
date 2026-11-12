package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
)

func gatedTool() Tool {
	return Tool{Name: "record_expense", RequireApproval: true, FreeWhen: []FreeCondition{
		{Arg: "confirm", Values: []json.RawMessage{json.RawMessage(`false`)}, Absent: true},
		{Arg: "mode", Values: []json.RawMessage{json.RawMessage(`"draft"`), json.RawMessage(`{"a":1,"b":2}`)}},
	}}
}

func TestFreeWhen_RoundTripsThroughProtoAndJSON(t *testing.T) {
	cfg := Config{Model: "m", ProviderID: uuid.New(), MCPServers: []MCPGrant{{ServerID: uuid.New(), Tools: []Tool{gatedTool()}}}}
	pb := configToProto(cfg)
	got := pb.GetMcpServers()[0].GetTools()[0].GetFreeWhen()
	require.Len(t, got, 2)
	assert.Equal(t, "confirm", got[0].GetArg())
	assert.Equal(t, []string{"false"}, got[0].GetValues())
	assert.True(t, got[0].GetAbsent())
	assert.Equal(t, []string{`"draft"`, `{"a":1,"b":2}`}, got[1].GetValues())

	back := toolsFromProto(pb.GetMcpServers()[0].GetTools())
	assert.True(t, back[0].Equal(gatedTool()))

	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"free_when":[{"arg":"confirm","values":[false],"absent":true}`)
	var again Config
	require.NoError(t, json.Unmarshal(raw, &again))
	assert.True(t, cfg.Equal(again))

	plain, err := json.Marshal(Tool{Name: "search"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"search"}`, string(plain), "a tool without free_when stores as before")
}

func TestFreeWhen_ProtoValuesAreStoredCanonical(t *testing.T) {
	tools := toolsFromProto([]*agentv1.AgentTool{{Name: "t", RequireApproval: true, FreeWhen: []*agentv1.FreeCondition{
		{Arg: "opts", Values: []string{` { "b" : 2, "a" : 1 } `, ` false `}},
	}}})
	assert.Equal(t, []json.RawMessage{json.RawMessage(`{"a":1,"b":2}`), json.RawMessage(`false`)}, tools[0].FreeWhen[0].Values,
		"key order and spacing never make two equal rules differ")
	bad := toolsFromProto([]*agentv1.AgentTool{{Name: "t", FreeWhen: []*agentv1.FreeCondition{{Arg: "a", Values: []string{"fals"}}}}})
	assert.Equal(t, json.RawMessage(`fals`), bad[0].FreeWhen[0].Values[0], "a value that is not JSON is kept for Validate to refuse")
}

func TestFreeWhen_Equal(t *testing.T) {
	assert.True(t, gatedTool().Equal(gatedTool()))
	for name, edit := range map[string]func(*Tool){
		"arg":             func(tl *Tool) { tl.FreeWhen[0].Arg = "dry_run" },
		"value":           func(tl *Tool) { tl.FreeWhen[0].Values[0] = json.RawMessage(`true`) },
		"absent":          func(tl *Tool) { tl.FreeWhen[0].Absent = false },
		"one value fewer": func(tl *Tool) { tl.FreeWhen[1].Values = tl.FreeWhen[1].Values[:1] },
		"one condition":   func(tl *Tool) { tl.FreeWhen = tl.FreeWhen[:1] },
		"none":            func(tl *Tool) { tl.FreeWhen = nil },
	} {
		tl := gatedTool()
		edit(&tl)
		assert.False(t, gatedTool().Equal(tl), name)
		a := Config{Tools: []Tool{gatedTool()}}
		b := Config{Tools: []Tool{tl}}
		assert.False(t, a.Equal(b), "config tools: %s", name)
		ga := Config{MCPServers: []MCPGrant{{Tools: []Tool{gatedTool()}}}}
		gb := Config{MCPServers: []MCPGrant{{Tools: []Tool{tl}}}}
		assert.False(t, ga.Equal(gb), "grant tools: %s", name)
	}
}

func TestFreeWhen_Validate(t *testing.T) {
	ok := Config{Model: "m", Tools: []Tool{gatedTool()}, MCPServers: []MCPGrant{{ServerID: uuid.New(), Tools: []Tool{gatedTool()}}}}
	require.NoError(t, ok.Validate())
	values := func(n int) []json.RawMessage {
		out := make([]json.RawMessage, n)
		for i := range out {
			out[i] = json.RawMessage(`1`)
		}
		return out
	}
	for name, tc := range map[string]struct {
		cond   []FreeCondition
		reason apperror.FieldReason
	}{
		"9 conditions": {func() []FreeCondition {
			c := make([]FreeCondition, 9)
			for i := range c {
				c[i] = FreeCondition{Arg: "a", Absent: true}
			}
			return c
		}(), apperror.FieldTooMany},
		"empty arg":        {[]FreeCondition{{Arg: "", Absent: true}}, apperror.FieldInvalidFormat},
		"arg with a dash":  {[]FreeCondition{{Arg: "dry-run", Absent: true}}, apperror.FieldInvalidFormat},
		"arg from a digit": {[]FreeCondition{{Arg: "1st", Absent: true}}, apperror.FieldInvalidFormat},
		"arg of 65":        {[]FreeCondition{{Arg: "a" + strings.Repeat("b", 64), Absent: true}}, apperror.FieldInvalidFormat},
		"17 values":        {[]FreeCondition{{Arg: "a", Values: values(17)}}, apperror.FieldTooMany},
		"a 257-byte value": {[]FreeCondition{{Arg: "a", Values: []json.RawMessage{json.RawMessage(`"` + strings.Repeat("x", 255) + `"`)}}}, apperror.FieldTooLong},
		"a value not JSON": {[]FreeCondition{{Arg: "a", Values: []json.RawMessage{json.RawMessage(`fals`)}}}, apperror.FieldInvalidFormat},
	} {
		for where, cfg := range map[string]Config{
			"config.tools.free_when":             {Model: "m", Tools: []Tool{{Name: "t", RequireApproval: true, FreeWhen: tc.cond}}},
			"config.mcp_servers.tools.free_when": {Model: "m", MCPServers: []MCPGrant{{ServerID: uuid.New(), Tools: []Tool{{Name: "t", RequireApproval: true, FreeWhen: tc.cond}}}}},
		} {
			err := cfg.Validate()
			ae, isApp := apperror.AsAppError(err)
			require.True(t, isApp, "%s at %s: %v", name, where, err)
			require.NotEmpty(t, ae.Violations(), name)
			assert.Equal(t, where, ae.Violations()[0].Field, name)
			assert.Equal(t, tc.reason, ae.Violations()[0].Reason, "%s at %s", name, where)
		}
	}
}

// SECURITY: a looser free_when on a gated tool lets calls skip the person, so it widens a bound grant like turning approval off.
func TestWidens_FreeWhenOnAGatedTool(t *testing.T) {
	gated := MCPGrant{Tools: []Tool{{Name: "record_expense", RequireApproval: true}}}
	loosened := MCPGrant{Tools: []Tool{gatedTool()}}
	assert.True(t, widens(gated, loosened), "adding free_when widens")
	changed := MCPGrant{Tools: []Tool{gatedTool()}}
	changed.Tools[0].FreeWhen[0].Values = append(changed.Tools[0].FreeWhen[0].Values, json.RawMessage(`"maybe"`))
	assert.True(t, widens(loosened, changed), "a changed free_when widens")
	assert.False(t, widens(loosened, MCPGrant{Tools: []Tool{gatedTool()}}), "the same free_when changes nothing")
	assert.False(t, widens(loosened, gated), "dropping free_when asks every time, which narrows")
	assert.False(t, widens(MCPGrant{Tools: []Tool{{Name: "record_expense"}}}, loosened), "an ungated tool becoming gated narrows")
}
