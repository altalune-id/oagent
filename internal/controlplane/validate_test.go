package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/kernel/session"
)

// NOTE: testdata/rule_reasons.json is the one reason table; the console tests its reasonOf against it too.
type ruleReason struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
	Param  string `json:"param"`
}

func ruleReasons(t *testing.T) []ruleReason {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "rule_reasons.json"))
	require.NoError(t, err)
	var rows []ruleReason
	require.NoError(t, json.Unmarshal(raw, &rows))
	return rows
}

func TestReasonOf(t *testing.T) {
	for _, row := range ruleReasons(t) {
		t.Run(row.Rule, func(t *testing.T) {
			reason, param := reasonOf(row.Rule)
			assert.Equal(t, apperror.FieldReason(row.Reason), reason)
			assert.Equal(t, row.Param, param)
		})
	}
}

func TestRuleReasonsCoverEveryCustomRule(t *testing.T) {
	listed := map[string]bool{}
	for _, row := range ruleReasons(t) {
		listed[row.Rule] = true
	}
	for _, id := range customRuleIDs(t) {
		assert.True(t, listed[id], "%s is used by a request but missing from testdata/rule_reasons.json", id)
	}
}

func fieldPath(elems ...*validate.FieldPathElement) *validate.FieldPath {
	return &validate.FieldPath{Elements: elems}
}

func named(name string) *validate.FieldPathElement {
	return &validate.FieldPathElement{FieldName: new(name)}
}

func indexed(name string, i uint64) *validate.FieldPathElement {
	return &validate.FieldPathElement{FieldName: new(name), Subscript: &validate.FieldPathElement_Index{Index: i}}
}

func TestViolationsOf(t *testing.T) {
	const secret = "hunter2-s3cret@example.com"
	verr := &protovalidate.ValidationError{Violations: []*protovalidate.Violation{
		{
			Proto:      &validate.Violation{Field: fieldPath(indexed("members", 0), named("role")), RuleId: new("string.in"), Message: new(secret)},
			FieldValue: protoreflect.ValueOfString(secret),
		},
		{
			Proto:      &validate.Violation{Field: fieldPath(named("name")), RuleId: new("string.max_len")},
			FieldValue: protoreflect.ValueOfString(secret),
			RuleValue:  protoreflect.ValueOfUint64(200),
		},
		{
			Proto:     &validate.Violation{Field: fieldPath(named("tags")), RuleId: new("repeated.max_items")},
			RuleValue: protoreflect.ValueOfUint64(10),
		},
		{
			Proto:     &validate.Violation{Field: fieldPath(named("password")), RuleId: new("string.const")},
			RuleValue: protoreflect.ValueOfString(secret),
		},
	}}

	got := violationsOf(verr)

	assert.Equal(t, []apperror.Violation{
		{Field: "members[0].role", Reason: apperror.FieldNotAllowed},
		{Field: "name", Reason: apperror.FieldTooLong, Param: "200"},
		{Field: "tags", Reason: apperror.FieldTooMany, Param: "10"},
		{Field: "password", Reason: apperror.FieldNotAllowed},
	}, got)
}

func newTestValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := NewValidator()
	require.NoError(t, err)
	return v
}

func TestValidate_PassesAValidRequest(t *testing.T) {
	assert.NoError(t, newTestValidator(t).Validate(&orgv1.CreateOrgRequest{Name: "Acme"}))
}

func TestValidate_RefusesCreateOrgName(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantReason apperror.FieldReason
		wantParam  string
	}{
		{"missing", "", apperror.FieldRequired, ""},
		{"blank", "   ", apperror.FieldRequired, ""},
		{"201 characters", strings.Repeat("a", 201), apperror.FieldTooLong, "200"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newTestValidator(t).Validate(&orgv1.CreateOrgRequest{Name: tt.value})
			ae, ok := apperror.AsAppError(err)
			require.True(t, ok, "want an AppError, got %v", err)
			assert.Equal(t, apperror.CodeValidation, ae.Code())
			assert.Equal(t, "name", ae.Meta()["field"])
			assert.Equal(t, string(tt.wantReason), ae.Meta()["reason"])
			assert.Equal(t, []apperror.Violation{{Field: "name", Reason: tt.wantReason, Param: tt.wantParam}}, ae.Violations())
		})
	}
}

func TestValidate_AcceptsAPaddedNameTheDomainAccepts(t *testing.T) {
	assert.NoError(t, newTestValidator(t).Validate(&orgv1.CreateOrgRequest{Name: "  " + strings.Repeat("a", 200) + "  "}))
}

func TestValidate_NeverEchoesTheValue(t *testing.T) {
	secret := "s3cret-" + strings.Repeat("x", 200)
	err := newTestValidator(t).Validate(&orgv1.CreateOrgRequest{Name: secret})
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.NotContains(t, ae.Error(), "s3cret")
	for k, v := range ae.Meta() {
		assert.NotContains(t, v, "s3cret", "meta %s", k)
	}
	for _, v := range ae.Violations() {
		assert.NotContains(t, v.Field+string(v.Reason)+v.Param, "s3cret")
	}
}

func TestValidate_WrapsAnUnexpectedFailure(t *testing.T) {
	v := &Validator{pv: failingValidator{}}
	err := v.Validate(&orgv1.CreateOrgRequest{})
	_, isApp := apperror.AsAppError(err)
	assert.False(t, isApp)
	assert.ErrorIs(t, err, errBroken)
}

var errBroken = errors.New("broken")

type failingValidator struct{}

func (failingValidator) Validate(proto.Message, ...protovalidate.ValidationOption) error {
	return errBroken
}

func detailOf(t *testing.T, err error) *apperrorv1.ErrorDetail {
	t.Helper()
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	for _, d := range cerr.Details() {
		v, derr := d.Value()
		require.NoError(t, derr)
		if det, ok := v.(*apperrorv1.ErrorDetail); ok {
			return det
		}
	}
	t.Fatalf("no ErrorDetail on %v", err)
	return nil
}

func TestAPI_RefusesAnInvalidRequestWithItsViolations(t *testing.T) {
	url := authServer(t, session.Principal{})
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, url).
		CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Name: strings.Repeat("a", 201)}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	det := detailOf(t, err)
	assert.Equal(t, string(apperror.CodeValidation), det.GetCode())
	assert.Equal(t, map[string]string{"field": "name", "reason": "too_long", "param": "200"}, det.GetMeta())
	require.Len(t, det.GetViolations(), 1)
	assert.Equal(t, "name", det.GetViolations()[0].GetField())
	assert.Equal(t, "too_long", det.GetViolations()[0].GetReason())
	assert.Equal(t, "200", det.GetViolations()[0].GetParam())
}

func TestAPI_AuthenticatesBeforeValidating(t *testing.T) {
	url := authServer(t, session.Principal{})
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, url).
		CreateOrg(context.Background(), bearer(connect.NewRequest(&orgv1.CreateOrgRequest{Name: strings.Repeat("a", 201)}), "oagent_bad"))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	assert.Empty(t, detailOf(t, err).GetViolations())
}

func TestAPI_ChecksPermissionBeforeValidating(t *testing.T) {
	p := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: uuid.New(), AllProjects: true, Scopes: []string{"agent:read"}}
	url := authServer(t, p)
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, url).
		CreateOrg(context.Background(), bearer(connect.NewRequest(&orgv1.CreateOrgRequest{Name: strings.Repeat("a", 201)}), "oagent_good"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.Empty(t, detailOf(t, err).GetViolations())
}

func TestValidate_NilValidatorStillRefuses(t *testing.T) {
	var v *Validator
	_, ok := apperror.AsAppError(v.Validate(&orgv1.CreateOrgRequest{}))
	assert.True(t, ok)
}

func TestAPI_PersonOnlyRefusesAKeyBeforeValidating(t *testing.T) {
	p := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: uuid.New(), AllProjects: true, Scopes: []string{"agent:read"}}
	url := authServer(t, p)
	_, err := authv1connect.NewAuthServiceClient(http.DefaultClient, url).
		ChangePassword(context.Background(), bearer(connect.NewRequest(&authv1.ChangePasswordRequest{}), "oagent_good"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.Empty(t, detailOf(t, err).GetViolations())
}
