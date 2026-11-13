package modelprovider

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
)

func TestProviderRow_ToProvider(t *testing.T) {
	row := providerRow{
		ID: uuid.New(), PublicID: "p", OrgID: uuid.New(), ProjectID: uuid.New(), Name: "n",
		Kind: "openai_compatible", BaseURL: "https://e.com", APIKeySealed: []byte{1},
		Models:    `[{"id":"m","vision":true,"prices":{"input_micros":1,"output_micros":2,"cache_read_micros":3,"cache_write_micros":4}},{"id":"u"},{"id":"e","embedding":true}]`,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}
	p, err := row.toProvider()
	require.NoError(t, err)
	assert.Equal(t, []Model{{ID: "m", Vision: true, Prices: &Prices{1, 2, 3, 4}}, {ID: "u"}, {ID: "e", Embedding: true}}, p.Models)
	assert.Equal(t, KindOpenAICompatible, p.Kind)
	assert.Equal(t, time.UTC, p.CreatedAt.Location())

	row.Models = "not json"
	_, err = row.toProvider()
	assert.Error(t, err)
	_, err = toProviders([]providerRow{row})
	assert.Error(t, err)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "", "oa_"))
}
