package db

import "database/sql"

// Pool bundles writer (W) and reader (R) handles; R aliases W when there is no reader.
type Pool struct {
	W *sql.DB
	R *sql.DB
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
