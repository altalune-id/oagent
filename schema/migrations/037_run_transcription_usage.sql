-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN transcription_seconds BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN transcription_price_micros BIGINT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP COLUMN IF EXISTS transcription_price_micros,
  DROP COLUMN IF EXISTS transcription_seconds;

-- +goose StatementEnd
