package agent

import (
	"encoding/json"

	"altalune.id/oagent/canonjson"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
)

func freeWhenToProto(conds []FreeCondition) []*agentv1.FreeCondition {
	if len(conds) == 0 {
		return nil
	}
	out := make([]*agentv1.FreeCondition, 0, len(conds))
	for _, c := range conds {
		values := make([]string, 0, len(c.Values))
		for _, v := range c.Values {
			values = append(values, string(v))
		}
		out = append(out, &agentv1.FreeCondition{Arg: c.Arg, Values: values, Absent: c.Absent})
	}
	return out
}

func freeWhenFromProto(conds []*agentv1.FreeCondition) []FreeCondition {
	if len(conds) == 0 {
		return nil
	}
	out := make([]FreeCondition, 0, len(conds))
	for _, c := range conds {
		var values []json.RawMessage
		for _, v := range c.GetValues() {
			// NOTE: stored canonical, so Equal and the publish diff never see key order; text that is not JSON is kept for Validate to refuse.
			raw, err := canonjson.Of([]byte(v))
			if err != nil {
				raw = json.RawMessage(v)
			}
			values = append(values, raw)
		}
		out = append(out, FreeCondition{Arg: c.GetArg(), Values: values, Absent: c.GetAbsent()})
	}
	return out
}
