package apikey

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
)

func TestRow_ToKey(t *testing.T) {
	hash := sha256.Sum256([]byte("secret"))
	pid := uuid.New()
	createdBy := uuid.New()
	in := apiKeyRow{
		ID:          uuid.New(),
		OrgID:       uuid.New(),
		ProjectID:   &pid,
		Kind:        string(KindOrg),
		AllProjects: true,
		ProjectIDs:  "{366d1b02-30b8-4b9f-9a34-8f8b6f1f5c0a}",
		Name:        "n",
		SecretHash:  hash[:],
		SecretHint:  "cret",
		Scopes:      "{org:read,agent:write}",
		ResourceIDs: "{}",
		CreatedBy:   &createdBy,
		CreatedAt:   time.Now(),
	}

	k, err := in.toKey()
	require.NoError(t, err)
	assert.Equal(t, KindOrg, k.Kind)
	assert.True(t, k.Matches("secret"))
	assert.Equal(t, []string{"org:read", "agent:write"}, k.Scopes)
	assert.Len(t, k.ProjectIDs, 1)
	assert.Equal(t, pid, k.ProjectID)
	assert.Equal(t, createdBy, k.CreatedBy)
	assert.True(t, k.AllProjects)
}

func TestPgArrayLiteral(t *testing.T) {
	assert.Equal(t, "{}", pgTextArrayLiteral(nil))
	assert.Equal(t, `{"a","b"}`, pgTextArrayLiteral([]string{"a", "b"}))
	assert.Equal(t, `{"a\"b"}`, pgTextArrayLiteral([]string{`a"b`}))
	assert.Equal(t, `{"`+"366d1b02-30b8-4b9f-9a34-8f8b6f1f5c0a"+`"}`, pgUUIDArrayLiteral([]uuid.UUID{uuid.MustParse("366d1b02-30b8-4b9f-9a34-8f8b6f1f5c0a")}))
}

func TestParsePgTextArray(t *testing.T) {
	out, err := parsePgTextArray("{a,b}")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, out)

	out, err = parsePgTextArray("{}")
	require.NoError(t, err)
	assert.Empty(t, out)

	out, err = parsePgTextArray(`{"a,b","c"}`)
	require.NoError(t, err)
	assert.Equal(t, []string{"a,b", "c"}, out)

	_, err = parsePgTextArray("not-an-array")
	assert.Error(t, err)
}

func TestNewStore(t *testing.T) {
	assert.NotNil(t, NewStore(db.Pool{}, "public", ""))
	assert.NotNil(t, NewStore(db.Pool{}, "", ""))
}

func apiKeyColumns() []string {
	return []string{
		"api_keys.id", "api_keys.org_id", "api_keys.project_id", "api_keys.kind", "api_keys.all_projects",
		"api_keys.project_ids", "api_keys.name", "api_keys.secret_hash", "api_keys.secret_hint",
		"api_keys.scopes", "api_keys.resource_ids", "api_keys.created_by", "api_keys.created_at",
		"api_keys.expires_at", "api_keys.revoked_at", "api_keys.last_used_at",
	}
}

func TestList(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	rows := sqlmock.NewRows(apiKeyColumns()).AddRow(
		uuid.New(), uuid.New(), nil, "org", true, "{}", "n", []byte{1, 2}, "hint", "{}", "{}", nil,
		time.Now(), nil, nil, nil,
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "")
	got, err := s.List(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n", got[0].Name)
}

func TestByID_NotFound(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(apiKeyColumns()))

	s := NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "")
	_, err = s.ByID(context.Background(), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestSave(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	s := NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "")
	k, _, err := NewScheme("").MintProject(uuid.New(), uuid.New(), "ci", nil, nil, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.NoError(t, s.Save(context.Background(), k))
}

func TestSave_OrgGrants(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	s := NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "")
	k, _, err := NewScheme("").MintOrg(uuid.New(), "deploy", nil, ProjectGrant{ProjectIDs: []uuid.UUID{uuid.New()}}, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.NoError(t, s.Save(context.Background(), k))
}

func TestStore_QueryErrors(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	require.NoError(t, sqlDB.Close())

	s := NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "")
	ctx := context.Background()
	_, err = s.ByID(ctx, uuid.New())
	assert.Error(t, err)
	_, err = s.List(ctx, uuid.New())
	assert.Error(t, err)

	k, _, err := NewScheme("").MintProject(uuid.New(), uuid.New(), "ci", nil, nil, future(), time.Now().UTC())
	require.NoError(t, err)
	assert.Error(t, s.Save(ctx, k))
}
