package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"altalune.id/oagent/apperror"
)

// Validator checks requests against the rules declared in their protos. A nil Validator uses protovalidate's shared one.
type Validator struct {
	pv protovalidate.Validator
}

// NewValidator builds the request Validator.
func NewValidator() (*Validator, error) {
	pv, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("building the request validator: %w", err)
	}
	return &Validator{pv: pv}, nil
}

// Validate returns a CodeValidation *apperror.AppError naming every refused field, or nil when msg follows its rules.
func (v *Validator) Validate(msg proto.Message) error {
	pv := protovalidate.GlobalValidator
	if v != nil {
		pv = v.pv
	}
	err := pv.Validate(msg)
	if err == nil {
		return nil
	}
	if verr, ok := errors.AsType[*protovalidate.ValidationError](err); ok {
		return apperror.Validation(violationsOf(verr))
	}
	return fmt.Errorf("validating %s: %w", msg.ProtoReflect().Descriptor().FullName(), err)
}

func (v *Validator) interceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if msg, ok := req.Any().(proto.Message); ok {
				if err := v.Validate(msg); err != nil {
					return nil, err
				}
			}
			return next(ctx, req)
		}
	}
}

// SECURITY: reads the rule and the field path only; FieldValue and Message may hold what the caller sent.
func violationsOf(verr *protovalidate.ValidationError) []apperror.Violation {
	out := make([]apperror.Violation, 0, len(verr.Violations))
	for _, pv := range verr.Violations {
		reason, param := reasonOf(pv.Proto.GetRuleId())
		if param == "" && hasLimit(reason) {
			param = limitOf(pv.RuleValue)
		}
		out = append(out, apperror.Violation{
			Field:  protovalidate.FieldPathString(pv.Proto.GetField()),
			Reason: reason,
			Param:  param,
		})
	}
	return out
}

func hasLimit(r apperror.FieldReason) bool {
	return r == apperror.FieldTooShort || r == apperror.FieldTooLong || r == apperror.FieldTooMany
}

func limitOf(v protoreflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	switch n := v.Interface().(type) {
	case uint64:
		return strconv.FormatUint(n, 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case uint32:
		return strconv.FormatUint(uint64(n), 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	}
	return ""
}

func reasonOf(ruleID string) (reason apperror.FieldReason, param string) {
	id := ruleID
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		if _, err := strconv.ParseUint(id[i+1:], 10, 64); err == nil {
			id, param = id[:i], id[i+1:]
		}
	}
	rule := id[strings.LastIndexByte(id, '.')+1:]
	switch {
	case rule == "required", rule == "blank", rule == "min_items", strings.HasSuffix(rule, "_empty"):
		return apperror.FieldRequired, param
	case rule == "min_len", rule == "min_bytes":
		return apperror.FieldTooShort, param
	case rule == "max_len", rule == "max_bytes":
		return apperror.FieldTooLong, param
	case rule == "in", rule == "not_in", rule == "const", rule == "defined_only":
		return apperror.FieldNotAllowed, param
	case rule == "max_items", rule == "max_pairs":
		return apperror.FieldTooMany, param
	case formatRules[rule]:
		return apperror.FieldInvalidFormat, param
	}
	return apperror.FieldInvalid, param
}

//nolint:gochecknoglobals // a fixed lookup table.
var formatRules = map[string]bool{
	"len": true, "len_bytes": true, "pattern": true, "prefix": true, "suffix": true, "contains": true, "not_contains": true,
	"email": true, "hostname": true, "ip": true, "ipv4": true, "ipv6": true, "uri": true, "uri_ref": true,
	"address": true, "uuid": true, "tuuid": true, "ulid": true, "host_and_port": true, "well_known_regex": true,
	"format": true, "public_id": true,
}
