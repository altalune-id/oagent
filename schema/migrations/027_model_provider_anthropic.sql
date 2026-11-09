-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers
  DROP CONSTRAINT {{.TablePrefix}}model_providers_kind_check;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers
  ADD CONSTRAINT {{.TablePrefix}}model_providers_kind_check CHECK (kind IN ('openai_compatible', 'anthropic'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers
  DROP CONSTRAINT {{.TablePrefix}}model_providers_kind_check;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}model_providers
  ADD CONSTRAINT {{.TablePrefix}}model_providers_kind_check CHECK (kind IN ('openai_compatible'));

-- +goose StatementEnd
