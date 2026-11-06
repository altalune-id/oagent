package mcpserver

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/db"
)

func TestServerRow_ToServer(t *testing.T) {
	header := "X-Api-Key"
	row := serverRow{
		ID: uuid.New(), PublicID: "p", OrgID: uuid.New(), ProjectID: uuid.New(), Name: "yasaku", URL: "https://e.com",
		AuthKind: "header", AuthHeader: &header, AuthSealed: []byte{1}, TimeoutMS: 5000, Trusted: true,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
	s := row.toServer()
	assert.Equal(t, AuthHeader, s.AuthKind)
	assert.Equal(t, "X-Api-Key", s.AuthHeader)
	assert.Equal(t, time.UTC, s.CreatedAt.Location())

	row.AuthHeader = nil
	assert.Empty(t, row.toServer().AuthHeader)
	assert.Len(t, toServers([]serverRow{row, row}), 2)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "", "oa_"))
}
