-- +goose Up
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  ADD COLUMN helper_input_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN helper_output_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN helper_cache_read_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN helper_cache_write_tokens BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN helper_price_input_micros BIGINT,
  ADD COLUMN helper_price_output_micros BIGINT,
  ADD COLUMN helper_price_cache_read_micros BIGINT,
  ADD COLUMN helper_price_cache_write_micros BIGINT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE {{.Schema}}.{{.TablePrefix}}runs
  DROP COLUMN IF EXISTS helper_price_cache_write_micros,
  DROP COLUMN IF EXISTS helper_price_cache_read_micros,
  DROP COLUMN IF EXISTS helper_price_output_micros,
  DROP COLUMN IF EXISTS helper_price_input_micros,
  DROP COLUMN IF EXISTS helper_cache_write_tokens,
  DROP COLUMN IF EXISTS helper_cache_read_tokens,
  DROP COLUMN IF EXISTS helper_output_tokens,
  DROP COLUMN IF EXISTS helper_input_tokens;

-- +goose StatementEnd
