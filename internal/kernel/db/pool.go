package db

import (
	"context"
	"database/sql"
	"log/slog"
)

// Pool bundles writer (W) and reader (R) handles; R aliases W when no reader DSN is set.
type Pool struct {
	W *sql.DB
	R *sql.DB
}

// OpenPool opens the writer and any configured reader.
func OpenPool(ctx context.Context, cfg DBConfig, log *slog.Logger) (Pool, error) {
	writer, err := Open(ctx, cfg, log)
	if err != nil {
		return Pool{}, err
	}
	p := Pool{W: writer, R: writer}
	if cfg.Reader.DSN == "" {
		return p, nil
	}
	readerCfg := cfg
	readerCfg.DSN = cfg.Reader.DSN
	readerCfg.Role = cfg.Reader.Role
	readerCfg.MaxOpenConns = cfg.Reader.MaxOpenConns
	readerCfg.MaxIdleConns = cfg.Reader.MaxIdleConns
	readerCfg.ConnMaxLifetime = cfg.Reader.ConnMaxLifetime
	readerCfg.ConnMaxIdleTime = cfg.Reader.ConnMaxIdleTime
	reader, rErr := Open(ctx, readerCfg, log)
	if rErr != nil {
		_ = p.Close()
		return Pool{}, rErr
	}
	p.R = reader
	return p, nil
}

// Close closes every distinct handle.
func (p Pool) Close() error {
	if p.R != nil && p.R != p.W {
		_ = p.R.Close()
	}
	if p.W != nil {
		return p.W.Close()
	}
	return nil
}
