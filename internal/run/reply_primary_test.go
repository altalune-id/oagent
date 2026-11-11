package run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/db/dbtest"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestReplyReadsUseThePrimary(t *testing.T) {
	ctx := tenant.WithOrg(context.Background(), uuid.New())
	for name, read := range map[string]func(s *postgresStore) error{
		"ReplyJob": func(s *postgresStore) error {
			_, err := s.ReplyJob(ctx, uuid.New())
			return err
		},
		"DueReplies": func(s *postgresStore) error {
			_, err := s.DueReplies(ctx, time.Now(), 10)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			w, wmock, err := sqlmock.New()
			require.NoError(t, err)
			r, _, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = w.Close()
				_ = r.Close()
			})
			dbtest.ExpectTx(wmock)
			wmock.ExpectQuery("SELECT").WillReturnError(errors.New("read on the primary"))
			wmock.ExpectRollback()

			err = read(NewStore(db.Pool{W: w, R: r}, "public", ""))
			assert.ErrorContains(t, err, "read on the primary", "a replica may lag behind the settle that made the reply due")
		})
	}
}
