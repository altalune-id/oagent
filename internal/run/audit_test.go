package run_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
)

type audit struct{ reader *sdkmetric.ManualReader }

func (f *svcFixture) withAudit(t *testing.T) *audit {
	t.Helper()
	a := &audit{reader: sdkmetric.NewManualReader()}
	f.svc.WithMeter(sdkmetric.NewMeterProvider(sdkmetric.WithReader(a.reader)).Meter("test"))
	return a
}

func (a *audit) mismatches(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, a.reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "approval.audit_mismatch" {
				continue
			}
			var n int64
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				n += dp.Value
			}
			return n
		}
	}
	return 0
}

func TestAudit_MismatchAlerts(t *testing.T) {
	gated := bytes.ReplaceAll(stream(t, "tool_call"), []byte("yasaku__record_expense"), []byte("yasaku__delete_all"))
	f := newSvc(t)
	audit := f.withAudit(t)
	f.say(t, "wipe it")
	f.execute(t, fakes.RuntimeReply{Stream: gated})
	assert.Equal(t, int64(1), audit.mismatches(t), "a gated call ran with no consumed approval")

	g := newSvc(t)
	clean := g.withAudit(t)
	g.say(t, "record lunch")
	g.execute(t, fakes.RuntimeReply{Stream: stream(t, "tool_call")})
	assert.Zero(t, clean.mismatches(t), "a free tool is never a mismatch")
}

func TestAudit_ApprovedCallIsNoMismatch(t *testing.T) {
	f := newSvc(t)
	f.version.MCPServers[0].Tools[0].RequireApproval = true
	f.version.MCPServers[0].Tools[1].RequireApproval = false
	audit := f.withAudit(t)
	a := f.withApprovals()
	parked := f.await(t, a, "call_1")
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: stream(t, "resume")})
	assert.Equal(t, "executed", a.CallState(parked.ID, "call_1"))
	assert.Zero(t, audit.mismatches(t), "the gated call ran on its consumed approval")
}

func TestAudit_NilMeterStillLogs(t *testing.T) {
	f := newSvc(t)
	f.svc.WithMeter(nil)
	f.say(t, "wipe it")
	f.execute(t, fakes.RuntimeReply{Stream: bytes.ReplaceAll(stream(t, "tool_call"), []byte("yasaku__record_expense"), []byte("yasaku__delete_all"))})
}

func TestAudit_CutLegIsAudited(t *testing.T) {
	gated := bytes.ReplaceAll(cut(stream(t, "tool_call")), []byte("yasaku__record_expense"), []byte("yasaku__delete_all"))
	f := newSvc(t)
	audit := f.withAudit(t)
	f.say(t, "wipe it")
	f.rt.Replies = append(f.rt.Replies, fakes.RuntimeReply{Stream: gated})
	token := f.claim(t)
	_, _ = f.svc.Execute(f.ctx, f.thread, token)
	require.NoError(t, f.store.Release(f.ctx, f.thread, token))
	assert.Equal(t, int64(1), audit.mismatches(t), "a leg that never finishes is audited at its checkpoint")
}

func TestAudit_ToolOutsideTheGrantsIsMismatch(t *testing.T) {
	stray := bytes.ReplaceAll(stream(t, "tool_call"), []byte("yasaku__record_expense"), []byte("stranger__drop_table"))
	f := newSvc(t)
	audit := f.withAudit(t)
	f.say(t, "hi")
	f.execute(t, fakes.RuntimeReply{Stream: stray})
	assert.Equal(t, int64(1), audit.mismatches(t), "a tool the agent was never granted is a mismatch")
}

func TestAudit_ApprovedCallOnAnAllGatedServerIsNoMismatch(t *testing.T) {
	onGated := func(raw []byte) []byte {
		return bytes.ReplaceAll(raw, []byte("yasaku__record_expense"), []byte("danger__wipe"))
	}
	f := newSvc(t)
	audit := f.withAudit(t)
	a := f.withApprovals()
	f.say(t, "wipe it")
	parked := f.execute(t, fakes.RuntimeReply{Stream: onGated(awaitStream(t, "call_1"))})
	require.Equal(t, run.StatusAwaitingApproval, parked.Status)
	a.Decide(parked.ID, "approved", "")
	f.execute(t, fakes.RuntimeReply{Stream: onGated(stream(t, "resume"))})
	assert.Equal(t, "executed", a.CallState(parked.ID, "call_1"))
	assert.Zero(t, audit.mismatches(t), "a server whose tools are all gated is never sent, but its tools are still granted")
}
