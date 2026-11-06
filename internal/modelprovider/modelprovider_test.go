package modelprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validInput() Input {
	return Input{
		Name:    "openrouter",
		Kind:    KindOpenAICompatible,
		BaseURL: "https://openrouter.ai/api/v1",
		Models: []Model{
			{ID: "gpt-4o", Vision: true, Prices: &Prices{InputMicros: 2_500_000, OutputMicros: 10_000_000}},
			{ID: "llama-3"},
		},
	}
}

func manyModels(n int) []Model {
	models := make([]Model, n)
	for i := range models {
		models[i] = Model{ID: fmt.Sprintf("m%d", i)}
	}
	return models
}

func TestNew(t *testing.T) {
	orgID, projectID := uuid.New(), uuid.New()
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

	p, err := New(orgID, projectID, validInput(), now)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, p.ID)
	assert.Len(t, p.PublicID, 21)
	assert.Equal(t, orgID, p.OrgID)
	assert.Equal(t, projectID, p.ProjectID)
	assert.Equal(t, "openrouter", p.Name)
	assert.Equal(t, KindOpenAICompatible, p.Kind)
	assert.Equal(t, "https://openrouter.ai/api/v1", p.BaseURL)
	assert.Equal(t, validInput().Models, p.Models)
	assert.Equal(t, now, p.CreatedAt)
	assert.Equal(t, now, p.UpdatedAt)
	assert.Empty(t, p.APIKeySealed)
}

func TestNew_Invalid(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(*Input)
		url    URLReason
		models ModelsReason
	}{
		{name: "http base url", edit: func(in *Input) { in.BaseURL = "http://llm.example.com/v1" }, url: URLHTTPSRequired},
		{name: "user info", edit: func(in *Input) { in.BaseURL = "https://user:pass@llm.example.com/v1" }, url: URLHasCredentials},
		{name: "empty base url", edit: func(in *Input) { in.BaseURL = "  " }, url: URLRequired},
		{name: "too long", edit: func(in *Input) { in.BaseURL = "https://e.com/" + strings.Repeat("a", MaxBaseURLLen) }, url: URLTooLong},
		{name: "not utf8", edit: func(in *Input) { in.BaseURL = "https://e.com/\xff" }, url: URLNotUTF8},
		{name: "unparsable", edit: func(in *Input) { in.BaseURL = "https://e.com/%zz" }, url: URLUnparsable},
		{name: "no host", edit: func(in *Input) { in.BaseURL = "https:///v1" }, url: URLNoHost},
		{name: "empty models", edit: func(in *Input) { in.Models = nil }, models: ModelsRequired},
		{name: "65 models", edit: func(in *Input) { in.Models = manyModels(MaxModels + 1) }, models: ModelsTooMany},
		{name: "duplicate model id", edit: func(in *Input) { in.Models = []Model{{ID: "a"}, {ID: "b"}, {ID: "a"}} }, models: ModelsDuplicate},
		{name: "negative input price", edit: func(in *Input) { in.Models[0].Prices.InputMicros = -1 }, models: ModelsPrice},
		{name: "negative output price", edit: func(in *Input) { in.Models[0].Prices.OutputMicros = -1 }, models: ModelsPrice},
		{name: "negative cache read price", edit: func(in *Input) { in.Models[0].Prices.CacheReadMicros = -1 }, models: ModelsPrice},
		{name: "negative cache write price", edit: func(in *Input) { in.Models[0].Prices.CacheWriteMicros = -1 }, models: ModelsPrice},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.edit(&in)
			p, err := New(uuid.New(), uuid.New(), in, time.Now())
			require.Error(t, err)
			assert.Nil(t, p)
			if tt.url != "" {
				var e *InvalidBaseURLError
				require.True(t, errors.As(err, &e), "got %v", err)
				assert.True(t, IsInvalidBaseURLError(err))
				assert.Equal(t, tt.url, e.Reason)
				return
			}
			var e *InvalidModelsError
			require.True(t, errors.As(err, &e), "got %v", err)
			assert.True(t, IsInvalidModelsError(err))
			assert.Equal(t, tt.models, e.Reason)
		})
	}
}

func TestNew_AllowInsecureURL(t *testing.T) {
	in := validInput()
	in.BaseURL = "http://127.0.0.1:11434/v1"
	p, err := New(uuid.New(), uuid.New(), in, time.Now(), AllowInsecureURL())
	require.NoError(t, err)
	assert.Equal(t, in.BaseURL, p.BaseURL)
}

func TestNew_SixtyFourModelsAllowed(t *testing.T) {
	in := validInput()
	in.Models = manyModels(MaxModels)
	_, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
}

func TestUpdate(t *testing.T) {
	created := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	p, err := New(uuid.New(), uuid.New(), validInput(), created)
	require.NoError(t, err)
	id, publicID := p.ID, p.PublicID
	p.APIKeySealed = []byte{1}

	later := created.Add(time.Hour)
	in := validInput()
	in.Name = "renamed"
	in.Models = []Model{{ID: "only"}}
	changed, err := p.Update(in, later)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, id, p.ID)
	assert.Equal(t, publicID, p.PublicID)
	assert.Equal(t, "renamed", p.Name)
	assert.Equal(t, []Model{{ID: "only"}}, p.Models)
	assert.Equal(t, created, p.CreatedAt)
	assert.Equal(t, later, p.UpdatedAt)
	assert.Equal(t, []byte{1}, p.APIKeySealed)

	in.BaseURL = "https://api.openai.com/v1"
	changed, err = p.Update(in, later.Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "https://api.openai.com/v1", p.BaseURL)
	assert.Equal(t, id, p.ID)
	assert.Equal(t, publicID, p.PublicID)
}

func TestUpdate_InvalidLeavesProviderUnchanged(t *testing.T) {
	created := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	p, err := New(uuid.New(), uuid.New(), validInput(), created)
	require.NoError(t, err)
	before := *p

	in := validInput()
	in.Name = "renamed"
	in.BaseURL = "http://llm.example.com"
	changed, err := p.Update(in, created.Add(time.Hour))
	assert.True(t, IsInvalidBaseURLError(err))
	assert.False(t, changed)
	assert.Equal(t, before, *p)
}

func TestModel(t *testing.T) {
	p, err := New(uuid.New(), uuid.New(), validInput(), time.Now())
	require.NoError(t, err)

	m, ok := p.Model("gpt-4o")
	require.True(t, ok)
	assert.True(t, m.Vision)
	require.NotNil(t, m.Prices)
	assert.Equal(t, int64(2_500_000), m.Prices.InputMicros)

	_, ok = p.Model("missing")
	assert.False(t, ok)
}

func TestModel_JSONShape(t *testing.T) {
	got, err := json.Marshal([]Model{
		{ID: "priced", Vision: true, Prices: &Prices{InputMicros: 1, OutputMicros: 2, CacheReadMicros: 3, CacheWriteMicros: 4}},
		{ID: "unpriced"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"id": "priced", "vision": true, "prices": {"input_micros": 1, "output_micros": 2, "cache_read_micros": 3, "cache_write_micros": 4}},
		{"id": "unpriced"}
	]`, string(got))
}

func TestNew_TrimsName(t *testing.T) {
	in := validInput()
	in.Name = "  openrouter  "
	p, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "openrouter", p.Name)

	in.Name = "\trenamed\n"
	_, err = p.Update(in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "renamed", p.Name)
}

func TestNew_CopiesPrices(t *testing.T) {
	in := validInput()
	p, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
	in.Models[0].Prices.InputMicros = 99
	in.Models[0].ID = "changed"
	assert.Equal(t, int64(2_500_000), p.Models[0].Prices.InputMicros)
	assert.Equal(t, "gpt-4o", p.Models[0].ID)

	upd := validInput()
	_, err = p.Update(upd, time.Now())
	require.NoError(t, err)
	upd.Models[0].Prices.OutputMicros = 99
	assert.Equal(t, int64(10_000_000), p.Models[0].Prices.OutputMicros)
}
