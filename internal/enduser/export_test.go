package enduser

import (
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ListSQL is the List statement's text and bound arguments, for the no-phone-in-spans test.
func ListSQL(query string) (string, []any) {
	return NewStore(db.Pool{}, "public", "").listStmt(uuid.New(), uuid.New(), query, Cursor{}, 10).Sql()
}
