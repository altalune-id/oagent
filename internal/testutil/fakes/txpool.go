package fakes

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"altalune.id/oagent/internal/kernel/db"
)

var errNoQueries = errors.New("fakes: TxPool runs no queries")

// TxPool returns a db.Pool whose transactions begin, commit and roll back but run no queries, for services over fake stores.
func TxPool() db.Pool {
	w := sql.OpenDB(txConnector{})
	return db.Pool{W: w, R: w}
}

type txConnector struct{}

func (txConnector) Connect(context.Context) (driver.Conn, error) { return txConn{}, nil }

func (txConnector) Driver() driver.Driver { return txDriver{} }

type txDriver struct{}

func (txDriver) Open(string) (driver.Conn, error) { return txConn{}, nil }

type txConn struct{}

func (txConn) Prepare(string) (driver.Stmt, error) { return nil, errNoQueries }

func (txConn) Close() error { return nil }

func (txConn) Begin() (driver.Tx, error) { return txConn{}, nil }

func (txConn) Commit() error { return nil }

func (txConn) Rollback() error { return nil }
