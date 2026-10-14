package protocmcp

import (
	"regexp"
	"strconv"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	// NOTE: registers the shared predefined rules so their field options parse.
	_ "altalune.id/oagent/gen/go/oagent/v1"
)

//nolint:gochecknoglobals // compiled once; a package-level regexp is the idiom.
var (
	maxLenID   = regexp.MustCompile(`\.max_len\.(\d+)$`)
	matchesCEL = regexp.MustCompile(`\bthis\.matches\('((?:[^'\\]|\\.)*)'\)`)
)

func fieldRules(field *protogen.Field) *validate.FieldRules {
	opts := field.Desc.Options()
	if opts == nil || !proto.HasExtension(opts, validate.E_Field) {
		return nil
	}
	rules, _ := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	return rules
}

func applyFieldRules(schema *jsonObject, field *protogen.Field, rules *validate.FieldRules) {
	if rules == nil {
		return
	}
	if field.Desc.IsList() {
		applyRepeatedRules(schema, rules.GetRepeated())
		return
	}
	if rules.GetRequired() && field.Desc.Kind() == protoreflect.StringKind {
		schema.put("minLength", 1)
	}
	applyStringRules(schema, rules.GetString())
	applyCELRules(schema, rules.GetCel())
}

func applyRepeatedRules(schema *jsonObject, r *validate.RepeatedRules) {
	if r == nil {
		return
	}
	if r.HasMinItems() {
		schema.put("minItems", r.GetMinItems())
	}
	if r.HasMaxItems() {
		schema.put("maxItems", r.GetMaxItems())
	}
	if r.GetUnique() {
		schema.put("uniqueItems", true)
	}
	if items, ok := schema.get("items").(*jsonObject); ok && r.GetItems() != nil {
		applyStringRules(items, r.GetItems().GetString())
		applyCELRules(items, r.GetItems().GetCel())
	}
}

func applyStringRules(schema *jsonObject, r *validate.StringRules) {
	if r == nil {
		return
	}
	if r.HasLen() {
		schema.put("minLength", r.GetLen()).put("maxLength", r.GetLen())
	}
	if r.HasMinLen() {
		schema.put("minLength", r.GetMinLen())
	}
	if r.HasMaxLen() {
		schema.put("maxLength", r.GetMaxLen())
	}
	if r.HasPattern() {
		schema.put("pattern", r.GetPattern())
	}
	if len(r.GetIn()) > 0 {
		schema.put("enum", r.GetIn())
	}
	switch {
	case r.GetEmail():
		schema.put("format", "email")
	case r.GetUri():
		schema.put("format", "uri")
	}
	r.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.IsExtension() && fd.Kind() == protoreflect.BoolKind && v.Bool() {
			applyPredefined(schema, fd)
		}
		return true
	})
}

func applyPredefined(schema *jsonObject, ext protoreflect.FieldDescriptor) {
	opts := ext.Options()
	if opts == nil || !proto.HasExtension(opts, validate.E_Predefined) {
		return
	}
	predefined, _ := proto.GetExtension(opts, validate.E_Predefined).(*validate.PredefinedRules)
	applyCELRules(schema, predefined.GetCel())
}

// NOTE: CEL rules map only by convention: an id ending max_len.N is maxLength N, and this.matches('re') on the raw value is its pattern.
func applyCELRules(schema *jsonObject, rules []*validate.Rule) {
	for _, rule := range rules {
		if m := maxLenID.FindStringSubmatch(rule.GetId()); m != nil {
			if n, err := strconv.ParseUint(m[1], 10, 64); err == nil {
				schema.put("maxLength", n)
			}
		}
		if m := matchesCEL.FindStringSubmatch(rule.GetExpression()); m != nil {
			schema.put("pattern", unescapeCEL(m[1]))
		}
	}
}

func unescapeCEL(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\'`, `'`).Replace(s)
}
