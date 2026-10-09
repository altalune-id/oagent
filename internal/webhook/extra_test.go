package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
)

func TestStoreErrorBranches(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	ctx := storeCtx(org, project)

	t.Run("byID query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.ByID(ctx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("list query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.List(ctx, org, project)
		assert.Error(t, err)
	})

	t.Run("list attempts query error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectQuery("SELECT").WillReturnError(errBoom)
		_, err := s.ListAttempts(ctx, uuid.New(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("save exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnError(errBoom)
		mock.ExpectRollback()
		e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, EventTypes: []events.Type{events.AgentCreated}}
		assert.Error(t, s.Save(ctx, e))
	})

	t.Run("saveSecrets exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.SaveSecrets(ctx, uuid.New(), SealedSecrets{}, SealedSecrets{Primary: []byte("n")}))
	})

	t.Run("delete exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})

	t.Run("saveAttempt exec error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT").WillReturnError(errBoom)
		mock.ExpectRollback()
		assert.Error(t, s.SaveAttempt(ctx, Attempt{OrgID: org, ProjectID: project, EndpointID: uuid.New()}))
	})

	t.Run("begin error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin().WillReturnError(errBoom)
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})

	t.Run("commit error", func(t *testing.T) {
		s, mock := newStoreTest(t)
		mock.ExpectBegin()
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errBoom)
		assert.Error(t, s.Delete(ctx, uuid.New()))
	})
}

func TestDelivererSaveAttemptError(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	ep.URL = srv.URL

	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}, attemptErr: errBoom}, sl, srv.Client(), nil)
	assert.NoError(t, d.Deliver(context.Background(), entryFor(ep, `{"type":"agent.created"}`)))
}

func TestDelivererExhausted(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	ep := sealedEndpoint(t, sl, org, project, "whsec_test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	ep.URL = srv.URL

	d := NewDeliverer(&fakeStore{endpoints: []*Endpoint{ep}}, sl, srv.Client(), nil)
	e := entryFor(ep, `{"type":"agent.created"}`)
	e.Attempt = outbox.MaxAttempts
	assert.True(t, IsDeliveryFailedError(d.Deliver(context.Background(), e)))
}

func TestTrimPartialRune(t *testing.T) {
	// "é" is two bytes; a cut inside it drops the partial rune.
	assert.Equal(t, []byte("a"), trimPartialRune([]byte("a\xc3")))
	assert.Equal(t, []byte("é"), trimPartialRune([]byte("é")))
	require.NotEmpty(t, trimPartialRune([]byte("abc")))
}

func TestServiceInsecureURL(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	svc := NewService(&fakeStore{}, nil, unexpectedStub, testSealer(t), &fakeOutbox{}, fakeSlugs{}, WithAllowInsecureURL())
	_, _, err := svc.Create(tenantCtx(org, project), "http://localhost:9999/hook", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)
}

func TestServiceCreateSaveError(t *testing.T) {
	svc := testService(t, &fakeStore{saveErr: errBoom}, &fakeOutbox{})
	_, _, err := svc.Create(tenantCtx(uuid.New(), uuid.New()), "https://e.com", "", []events.Type{events.AgentCreated})
	assert.Error(t, err)
}

func TestEnvelopeType(t *testing.T) {
	_, err := envelopeType([]byte("nope"))
	assert.Error(t, err)
	_, err = envelopeType([]byte(`{}`))
	assert.Error(t, err)
}

func TestRedactedErrorURL(t *testing.T) {
	err := &url.Error{Op: "Post", Err: errors.New("dial")}
	assert.Contains(t, redactedError(err), "Post")
}
