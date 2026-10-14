package config

import (
	"errors"
	"fmt"
	"net/mail"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

//nolint:gochecknoglobals // validator instance is stateless and safe for concurrent use.
var v10 = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	for tag, fn := range map[string]validator.Func{
		"mail_address": func(fl validator.FieldLevel) bool { return isMailAddress(fl.Field().String()) },
		"secret":       func(fl validator.FieldLevel) bool { return len(fl.Field().String()) >= minSecretLen },
	} {
		if err := v.RegisterValidation(tag, fn); err != nil {
			panic(err)
		}
	}
	return v
}

func isMailAddress(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil
}

func (c *Config) validateTags() error {
	err := v10.Struct(c)
	if err == nil {
		return nil
	}
	fieldErrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return fmt.Errorf("config: %w", err)
	}
	errs := make([]error, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		errs = append(errs, settingError(fe))
	}
	return errors.Join(errs...)
}

// SECURITY: never put fe.Value() in a reason; a field may hold a secret.
func settingError(fe validator.FieldError) error {
	_, key, _ := strings.Cut(fe.Namespace(), ".")
	if strings.HasPrefix(fe.Tag(), "required") {
		return &MissingSettingError{Keys: key, Reason: "must not be empty"}
	}
	return &InvalidSettingError{Key: key, Reason: tagReason(fe)}
}

func tagReason(fe validator.FieldError) string {
	param := fe.Param()
	unit := ""
	if fe.Kind() == reflect.String {
		unit = " characters"
	}
	switch fe.Tag() {
	case "oneof":
		return "must be one of " + strings.ReplaceAll(param, " ", ", ")
	case "gte", "min":
		if param == "0" {
			return "must be 0 or more"
		}
		return "must be at least " + param + unit
	case "lte", "max":
		return "must be at most " + param + unit
	case "url":
		return "must be an absolute URL"
	case "email":
		return "must be an email address"
	case "datetime":
		if param == time.DateOnly {
			return "must be a date as YYYY-MM-DD"
		}
		return "must be a date as " + param
	case "mail_address":
		return "must be an email address, optionally with a name: Name <addr@example.com>"
	case "secret":
		return fmt.Sprintf("must be at least %d characters; %s", minSecretLen, generateHint)
	default:
		return "is not valid"
	}
}
