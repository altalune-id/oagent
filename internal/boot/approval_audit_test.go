package boot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/run"
)

func TestRunApprovals_NeedsApproval(t *testing.T) {
	confirm := json.RawMessage(`[{"arg":"confirm","values":[false]}]`)
	for name, tc := range map[string]struct {
		grant run.ToolGrant
		args  string
		want  bool
	}{
		"free tool":              {run.ToolGrant{Name: "list"}, `{}`, false},
		"gated, no conditions":   {run.ToolGrant{Name: "wipe", RequireApproval: true}, `{}`, true},
		"gated, free_when holds": {run.ToolGrant{Name: "pay", RequireApproval: true, FreeWhen: confirm}, `{"confirm":false}`, false},
		"gated, free_when fails": {run.ToolGrant{Name: "pay", RequireApproval: true, FreeWhen: confirm}, `{"confirm":true}`, true},
		"args not JSON":          {run.ToolGrant{Name: "pay", RequireApproval: true, FreeWhen: confirm}, `not json`, true},
		"conditions not JSON":    {run.ToolGrant{Name: "pay", RequireApproval: true, FreeWhen: json.RawMessage(`{`)}, `{}`, true},
	} {
		assert.Equal(t, tc.want, runApprovals{}.NeedsApproval(tc.grant, json.RawMessage(tc.args)), name)
	}
}
