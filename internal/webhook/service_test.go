package webhook

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/sealer"
)

type fakeStore struct {
	Store
	endpoints    []*Endpoint
	saved        []*Endpoint
	deleted      []uuid.UUID
	savedSecrets []SealedSecrets
	attempts     []Attempt
	saveErr      error
	byIDErr      error
	listErr      error
	deleteErr    error
	secretsErr   error
	attemptErr   error
}

func (s *fakeStore) Save(_ context.Context, e *Endpoint) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, e)
	return nil
}

func (s *fakeStore) SaveSecrets(_ context.Context, _ uuid.UUID, _, next SealedSecrets) error {
	if s.secretsErr != nil {
		return s.secretsErr
	}
	s.savedSecrets = append(s.savedSecrets, next)
	return nil
}

func (s *fakeStore) ByID(_ context.Context, id uuid.UUID) (*Endpoint, error) {
	if s.byIDErr != nil {
		return nil, s.byIDErr
	}
	for _, e := range s.endpoints {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, &NotFoundError{ID: id.String()}
}

func (s *fakeStore) ByPublicID(_ context.Context, publicID string) (*Endpoint, error) {
	if s.byIDErr != nil {
		return nil, s.byIDErr
	}
	for _, e := range s.endpoints {
		if e.PublicID == publicID {
			return e, nil
		}
	}
	return nil, &NotFoundError{ID: publicID}
}

func (s *fakeStore) List(_ context.Context, orgID, projectID uuid.UUID) ([]*Endpoint, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := []*Endpoint{}
	for _, e := range s.endpoints {
		if e.OrgID == orgID && e.ProjectID == projectID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *fakeStore) ListByOrg(_ context.Context, orgID uuid.UUID) ([]*Endpoint, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := []*Endpoint{}
	for _, e := range s.endpoints {
		if e.OrgID == orgID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	for i, e := range s.endpoints {
		if e.ID == id {
			s.endpoints = append(s.endpoints[:i], s.endpoints[i+1:]...)
			s.deleted = append(s.deleted, id)
			return nil
		}
	}
	return &NotFoundError{ID: id.String()}
}

func (s *fakeStore) SaveAttempt(_ context.Context, a Attempt) error {
	if s.attemptErr != nil {
		return s.attemptErr
	}
	s.attempts = append(s.attempts, a)
	return nil
}

func (s *fakeStore) ListAttempts(context.Context, uuid.UUID, uuid.UUID) ([]Attempt, error) {
	if s.attemptErr != nil {
		return nil, s.attemptErr
	}
	return s.attempts, nil
}

type fakeOutbox struct {
	outbox.Store
	enqueued         []outbox.Entry
	entries          []outbox.Entry
	listErr          error
	requeueErr       error
	requeued         int
	requeueFailedErr error
}

func (f *fakeOutbox) Enqueue(_ context.Context, e outbox.Entry) error {
	f.enqueued = append(f.enqueued, e)
	return nil
}

func (f *fakeOutbox) ListByTarget(context.Context, string, int) ([]outbox.Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.entries, nil
}

func (f *fakeOutbox) IDByPublicID(_ context.Context, publicID, target string) (uuid.UUID, error) {
	if f.listErr != nil {
		return uuid.Nil, f.listErr
	}
	for _, e := range f.entries {
		if e.PublicID == publicID && e.Target == target {
			return e.ID, nil
		}
	}
	return uuid.Nil, &outbox.NotFoundError{ID: publicID}
}

func (f *fakeOutbox) Requeue(context.Context, uuid.UUID, string) error { return f.requeueErr }

func (f *fakeOutbox) RequeueFailed(context.Context, string) (int, error) {
	return f.requeued, f.requeueFailedErr
}

type fakeResolver struct{ err error }

func (f fakeResolver) ProjectRef(context.Context, uuid.UUID) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return "proj", "prj_pub", nil
}

func (f fakeResolver) OrgRef(context.Context, uuid.UUID) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "org_pub", nil
}

func unexpectedStub(_ context.Context, message string, cause error, _ ...any) *apperror.AppError {
	return apperror.New(apperror.CodeUnexpected, message).WithCause(cause)
}

func testService(t *testing.T, store Store, ob outbox.Store) *Service {
	t.Helper()
	sl := testSealer(t)
	return NewService(store, nil, unexpectedStub, sl, ob, fakeResolver{})
}

func tenantCtx(org, project uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: org, ProjectID: project})
}

func TestServiceCreate(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	store := &fakeStore{}
	svc := testService(t, store, &fakeOutbox{})

	e, secret, err := svc.Create(tenantCtx(org, project), "https://example.com", "d", []events.Type{events.AgentCreated})
	require.NoError(t, err)
	assert.Contains(t, secret, secretPrefix)
	assert.Equal(t, org, e.OrgID)
	assert.NotEmpty(t, e.Secrets.Primary)
	require.Len(t, store.saved, 1)
}

func TestServiceCreateLimit(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	store := &fakeStore{}
	for range MaxEndpointsPerProject {
		store.endpoints = append(store.endpoints, &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true})
	}
	svc := testService(t, store, &fakeOutbox{})
	_, _, err := svc.Create(tenantCtx(org, project), "https://example.com", "", []events.Type{events.AgentCreated})
	assert.True(t, IsEndpointLimitError(err))
}

func TestServiceCreateListError(t *testing.T) {
	store := &fakeStore{listErr: errors.New("boom")}
	svc := testService(t, store, &fakeOutbox{})
	_, _, err := svc.Create(tenantCtx(uuid.New(), uuid.New()), "https://example.com", "", []events.Type{events.AgentCreated})
	assert.Error(t, err)
}

func TestServiceCreateSealerUnavailable(t *testing.T) {
	svc := NewService(&fakeStore{}, nil, unexpectedStub, sealer.Disabled(), &fakeOutbox{}, fakeResolver{})
	_, _, err := svc.Create(tenantCtx(uuid.New(), uuid.New()), "https://example.com", "", []events.Type{events.AgentCreated})
	assert.True(t, sealer.IsUnavailableError(err))
}

func TestServiceList(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	store := &fakeStore{endpoints: []*Endpoint{{ID: uuid.New(), OrgID: org, ProjectID: project}}}
	svc := testService(t, store, &fakeOutbox{})
	got, err := svc.List(tenantCtx(org, project))
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestServiceByID(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{})

	got, err := svc.ByID(tenantCtx(org, project), e.ID)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)

	_, err = svc.ByID(tenantCtx(org, uuid.New()), e.ID)
	assert.True(t, IsNotFoundError(err))

	_, err = svc.ByID(tenantCtx(org, project), uuid.New())
	assert.True(t, IsNotFoundError(err))
}

func TestServiceUpdate(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, URL: "https://a.example.com", EventTypes: []events.Type{events.AgentCreated}}
	store := &fakeStore{endpoints: []*Endpoint{e}}
	svc := testService(t, store, &fakeOutbox{})

	got, err := svc.Update(tenantCtx(org, project), e.ID, "https://b.example.com", "d", []events.Type{events.AgentUpdated}, false)
	require.NoError(t, err)
	assert.Equal(t, "https://b.example.com", got.URL)
	require.Len(t, store.saved, 1)

	_, err = svc.Update(tenantCtx(org, project), e.ID, "", "d", []events.Type{events.AgentCreated}, true)
	assert.True(t, IsInvalidURLError(err))
}

func TestServiceDelete(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	store := &fakeStore{endpoints: []*Endpoint{e}}
	svc := testService(t, store, &fakeOutbox{})

	require.NoError(t, svc.Delete(tenantCtx(org, project), e.ID))
	assert.Len(t, store.deleted, 1)

	assert.True(t, IsNotFoundError(svc.Delete(tenantCtx(org, project), uuid.New())))
}

func TestServiceRotate(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	primary, err := sealSecret(sl, e.ID, slotPrimary, "whsec_old")
	require.NoError(t, err)
	e.Secrets = SealedSecrets{Primary: primary}
	store := &fakeStore{endpoints: []*Endpoint{e}}
	svc := NewService(store, nil, unexpectedStub, sl, &fakeOutbox{}, fakeResolver{})

	secret, err := svc.Rotate(tenantCtx(org, project), e.ID)
	require.NoError(t, err)
	assert.Contains(t, secret, secretPrefix)
	require.Len(t, store.savedSecrets, 1)
	assert.NotNil(t, store.savedSecrets[0].Secondary) // old primary kept as secondary
}

func TestServiceRotateConflict(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Secrets: SealedSecrets{Primary: []byte("junk")}}
	store := &fakeStore{endpoints: []*Endpoint{e}, secretsErr: &SecretConflictError{ID: e.ID.String()}}
	svc := NewService(store, nil, unexpectedStub, sl, &fakeOutbox{}, fakeResolver{})

	_, err := svc.Rotate(tenantCtx(org, project), e.ID)
	assert.True(t, IsSecretConflictError(err))
}

func TestServiceRetireSecondary(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	sl := testSealer(t)
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Secrets: SealedSecrets{Primary: []byte("p"), Secondary: []byte("s")}}
	store := &fakeStore{endpoints: []*Endpoint{e}}
	svc := NewService(store, nil, unexpectedStub, sl, &fakeOutbox{}, fakeResolver{})

	require.NoError(t, svc.RetireSecondary(tenantCtx(org, project), e.ID))
	require.Len(t, store.savedSecrets, 1)
	assert.Nil(t, store.savedSecrets[0].Secondary)

	// no-op when no secondary
	e.Secrets.Secondary = nil
	require.NoError(t, svc.RetireSecondary(tenantCtx(org, project), e.ID))
}

func TestServiceEnqueue(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	subscribed := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true, EventTypes: []events.Type{events.AgentCreated}}
	other := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true, EventTypes: []events.Type{events.AgentUpdated}}
	inactive := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: false, EventTypes: []events.Type{events.AgentCreated}}
	store := &fakeStore{endpoints: []*Endpoint{subscribed, other, inactive}}
	ob := &fakeOutbox{}
	svc := testService(t, store, ob)

	payload := events.AgentCreatedV1{Slug: "a"}
	require.NoError(t, svc.Enqueue(tenantCtx(org, project), events.AgentCreated, payload))
	require.Len(t, ob.enqueued, 1)
	assert.Equal(t, subscribed.ID.String(), ob.enqueued[0].Target)
}

func TestServiceEnqueueErrors(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	svc := testService(t, &fakeStore{}, &fakeOutbox{})

	assert.True(t, events.IsUnknownTypeError(svc.Enqueue(tenantCtx(org, project), "nope", 1)))
	assert.True(t, events.IsPayloadMismatchError(svc.Enqueue(tenantCtx(org, project), events.AgentCreated, 1)))
	assert.True(t, tenant.IsMissingError(svc.Enqueue(context.Background(), events.AgentCreated, events.AgentCreatedV1{})))

	bad := testService(t, &fakeStore{listErr: errors.New("boom")}, &fakeOutbox{})
	assert.Error(t, bad.Enqueue(tenantCtx(org, project), events.AgentCreated, events.AgentCreatedV1{}))
}

func TestServiceEnqueueSlugError(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true, EventTypes: []events.Type{events.AgentCreated}}
	store := &fakeStore{endpoints: []*Endpoint{e}}
	svc := NewService(store, nil, unexpectedStub, testSealer(t), &fakeOutbox{}, fakeResolver{err: errors.New("boom")})
	assert.Error(t, svc.Enqueue(tenantCtx(org, project), events.AgentCreated, events.AgentCreatedV1{}))
}

func TestServiceSendTest(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	active := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true}
	ob := &fakeOutbox{}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{active}}, ob)

	require.NoError(t, svc.SendTest(tenantCtx(org, project), active.ID))
	require.Len(t, ob.enqueued, 1)

	inactive := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: false}
	svc2 := testService(t, &fakeStore{endpoints: []*Endpoint{inactive}}, &fakeOutbox{})
	assert.True(t, IsEndpointInactiveError(svc2.SendTest(tenantCtx(org, project), inactive.ID)))
}

func TestServiceEnqueueOrg_FansOutPerProject(t *testing.T) {
	org, p1, p2 := uuid.New(), uuid.New(), uuid.New()
	a := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: p1, Active: true, EventTypes: []events.Type{events.MemberAdded}}
	b := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: p2, Active: true, EventTypes: []events.Type{events.MemberAdded, events.InviteCreated}}
	c := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: p2, Active: true, EventTypes: []events.Type{events.MemberAdded}}
	unsubscribed := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: p1, Active: true, EventTypes: []events.Type{events.AgentCreated}}
	inactive := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: p1, Active: false, EventTypes: []events.Type{events.MemberAdded}}
	elsewhere := &Endpoint{ID: uuid.New(), OrgID: uuid.New(), ProjectID: uuid.New(), Active: true, EventTypes: []events.Type{events.MemberAdded}}
	ob := &fakeOutbox{}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{a, b, c, unsubscribed, inactive, elsewhere}}, ob)

	require.NoError(t, svc.EnqueueOrg(context.Background(), org, events.MemberAdded, events.MemberAddedV1{UserID: "u"}))
	require.Len(t, ob.enqueued, 3)
	byProject := map[uuid.UUID][]string{}
	eventOf := map[uuid.UUID]uuid.UUID{}
	for _, e := range ob.enqueued {
		assert.Equal(t, org, e.OrgID)
		byProject[e.ProjectID] = append(byProject[e.ProjectID], e.Target)
		if prev, ok := eventOf[e.ProjectID]; ok {
			assert.Equal(t, prev, e.EventID, "one event per project")
		}
		eventOf[e.ProjectID] = e.EventID
	}
	assert.ElementsMatch(t, []string{a.ID.String()}, byProject[p1])
	assert.ElementsMatch(t, []string{b.ID.String(), c.ID.String()}, byProject[p2])
	assert.NotEqual(t, eventOf[p1], eventOf[p2])

	ob2 := &fakeOutbox{}
	quiet := testService(t, &fakeStore{endpoints: []*Endpoint{unsubscribed}}, ob2)
	require.NoError(t, quiet.EnqueueOrg(context.Background(), org, events.MemberAdded, events.MemberAddedV1{}))
	assert.Empty(t, ob2.enqueued)
}

func TestServiceEnqueueOrgErrors(t *testing.T) {
	org := uuid.New()
	svc := testService(t, &fakeStore{}, &fakeOutbox{})
	assert.True(t, events.IsUnknownTypeError(svc.EnqueueOrg(context.Background(), org, "nope", 1)))
	assert.True(t, events.IsPayloadMismatchError(svc.EnqueueOrg(context.Background(), org, events.InviteCreated, 1)))
	bad := testService(t, &fakeStore{listErr: errors.New("boom")}, &fakeOutbox{})
	assert.Error(t, bad.EnqueueOrg(context.Background(), org, events.InviteCreated, events.InviteCreatedV1{}))
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: uuid.New(), Active: true, EventTypes: []events.Type{events.InviteCreated}}
	refless := NewService(&fakeStore{endpoints: []*Endpoint{e}}, nil, unexpectedStub, testSealer(t), &fakeOutbox{}, fakeResolver{err: errors.New("boom")})
	assert.Error(t, refless.EnqueueOrg(context.Background(), org, events.InviteCreated, events.InviteCreatedV1{}))
}
