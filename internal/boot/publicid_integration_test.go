//go:build integration

package boot

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/permission"
)

var uuidShaped = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func fillProbe(msg *dynamicpb.Message) {
	fields := msg.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if msg.Has(fd) {
			continue
		}
		switch {
		case fd.IsList() && fd.Name() == "event_types":
			list := msg.Mutable(fd).List()
			list.Append(protoreflect.ValueOfString("agent.created"))
		case fd.IsList() && fd.Name() == "scopes":
			list := msg.Mutable(fd).List()
			list.Append(protoreflect.ValueOfString("agent:read"))
		case fd.Kind() == protoreflect.StringKind && !fd.IsList():
			if v, ok := probeStrings[fd.Name()]; ok {
				msg.Set(fd, protoreflect.ValueOfString(v))
			}
			if fd.Name() == "slug" {
				msg.Set(fd, protoreflect.ValueOfString("probe-"+strconv.FormatInt(time.Now().UnixNano(), 10)))
			}
		case fd.Kind() == protoreflect.BoolKind && (fd.Name() == "all_projects" || fd.Name() == "active"):
			msg.Set(fd, protoreflect.ValueOfBool(true))
		case fd.Kind() == protoreflect.MessageKind && fd.Name() == "expires_at":
			msg.Set(fd, protoreflect.ValueOfMessage(timestamppb.New(time.Now().Add(time.Hour)).ProtoReflect()))
		}
	}
}

var probeStrings = map[protoreflect.Name]string{
	"name": "Probe", "description": "probe", "url": "https://example.com/probe",
	"email": "probe@example.com", "role": "member", "kind": "org",
}

func (f *apiFixture) callForResponse(
	t *testing.T, procedure string, method protoreflect.MethodDescriptor, msg *dynamicpb.Message, header, value string,
) (*dynamicpb.Message, error) {
	t.Helper()
	client := connect.NewClient[dynamicpb.Message, dynamicpb.Message](http.DefaultClient, f.url+procedure,
		connect.WithSchema(method),
		connect.WithResponseInitializer(func(_ connect.Spec, out any) error {
			if m, ok := out.(*dynamicpb.Message); ok {
				*m = *dynamicpb.NewMessage(method.Output())
			}
			return nil
		}))
	req := connect.NewRequest(msg)
	req.Header().Set(header, value)
	resp, err := client.CallUnary(t.Context(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func errorText(t *testing.T, err error) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(err.Error())
	if cerr, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range cerr.Details() {
			if v, derr := d.Value(); derr == nil {
				b.WriteString(protojson.Format(v))
			}
		}
	}
	return b.String()
}

// Z14, Review Focus 1: no RPC answers with a UUID, as a caller who can see org B's data.
func TestPublicIDs_NoRPCAnswersWithAUUID(t *testing.T) {
	f := newAPIFixture(t)
	s := seedReach(t, f)
	cookie, _, _ := f.personCookieIn(t, s.orgB, "b-owner@example.com", member.RoleOwner)
	table := permission.Table()
	var answered []string
	for _, proc := range reachOrder() {
		method, msg, _ := s.request(t, proc)
		fillProbe(msg)
		resp, err := f.callForResponse(t, proc, method, msg, "Cookie", cookie)
		var body string
		if err != nil {
			body = errorText(t, err)
		} else {
			raw, merr := protojson.Marshal(resp)
			require.NoError(t, merr)
			body = string(raw)
			answered = append(answered, proc)
		}
		assert.Empty(t, uuidShaped.FindAllString(body, -1), "%s answered with a UUID: %s", proc, body)
	}
	for _, proc := range reachOrder() {
		if !slices.ContainsFunc(table[proc], func(p string) bool { return !strings.HasSuffix(p, ":read") }) {
			assert.Contains(t, answered, proc, "a read must succeed, or its response is never checked")
		}
	}
	assert.Greater(t, len(answered), len(table)/2, "most RPCs must succeed for the check to mean anything: %v", answered)
}
