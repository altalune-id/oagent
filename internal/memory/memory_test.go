package memory_test

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/memory"
)

var nativeOn = []memory.Backend{memory.BackendNative}

func codeOf(err error) apperror.Code {
	if ae, ok := apperror.AsAppError(err); ok {
		return ae.Code()
	}
	return ""
}

func missing(t *testing.T, err error, field string) {
	t.Helper()
	var e *memory.IncompleteScopeError
	if assert.ErrorAs(t, err, &e, field) {
		assert.Equal(t, field, e.Field)
	}
}

func TestScope_Check(t *testing.T) {
	full := memory.Scope{OrgID: uuid.New(), ProjectID: uuid.New(), StoreID: uuid.New(), AgentID: uuid.New(), EndUserID: uuid.New()}
	assert.NoError(t, full.Check(true))
	person := full
	person.AgentID = uuid.Nil
	assert.NoError(t, person.Check(false), "admin calls may span the person's agents")
	missing(t, person.Check(true), "agent")
	for field, drop := range map[string]func(*memory.Scope){
		"org":      func(s *memory.Scope) { s.OrgID = uuid.Nil },
		"project":  func(s *memory.Scope) { s.ProjectID = uuid.Nil },
		"store":    func(s *memory.Scope) { s.StoreID = uuid.Nil },
		"end_user": func(s *memory.Scope) { s.EndUserID = uuid.Nil },
	} {
		s := full
		drop(&s)
		missing(t, s.Check(false), field)
	}
}

func TestScope_CheckStore(t *testing.T) {
	store := memory.Scope{OrgID: uuid.New(), ProjectID: uuid.New(), StoreID: uuid.New()}
	assert.NoError(t, store.CheckStore(), "a whole store names no person or agent")
	for name, drop := range map[string]func(*memory.Scope){
		"org":     func(s *memory.Scope) { s.OrgID = uuid.Nil },
		"project": func(s *memory.Scope) { s.ProjectID = uuid.Nil },
		"store":   func(s *memory.Scope) { s.StoreID = uuid.Nil },
	} {
		s := store
		drop(&s)
		missing(t, s.CheckStore(), name)
	}
}

func TestCleanFacts(t *testing.T) {
	in := []string{"  Lives in Bandung.  ", "", "   ", strings.Repeat("é", 600)}
	for range 12 {
		in = append(in, "x")
	}
	out := memory.CleanFacts(in)
	assert.Len(t, out, runtime.MaxExtractedFacts)
	assert.Equal(t, "Lives in Bandung.", out[0])
	assert.Equal(t, strings.Repeat("é", runtime.MaxMemoryFactRunes), out[1], "cut to the rune cap, never a byte cap")
}

func TestCleanUserTexts(t *testing.T) {
	out := memory.CleanUserTexts([]string{strings.Repeat("a", 5000), " hi "})
	assert.Equal(t, []string{strings.Repeat("a", runtime.MaxUserTextRunes), "hi"}, out)
}

func TestHash_SameFactOnce(t *testing.T) {
	assert.Equal(t, memory.Hash("Lives in  Bandung."), memory.Hash(" lives IN bandung. "))
	assert.Equal(t, memory.Hash("ﬁle"), memory.Hash("file"), "NFKC folds compatibility forms")
	assert.NotEqual(t, memory.Hash("Lives in Bandung."), memory.Hash("Lives in Jakarta."))
}

func TestErrors_AreExpected(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &memory.NotFoundError{ID: "f1"})
	assert.True(t, memory.IsNotFoundError(err))
	assert.True(t, memory.IsExpectedError(err))
	assert.NotContains(t, err.Error(), "f1", "the message never echoes an id")
	assert.False(t, memory.IsExpectedError(&memory.IncompleteScopeError{Field: "agent"}), "an incomplete scope is a bug, reported")
	assert.False(t, memory.IsExpectedError(&memory.TenantMismatchError{}), "so is a scope of another tenant")
	assert.True(t, memory.IsTenantMismatchError(fmt.Errorf("w: %w", &memory.TenantMismatchError{})))
	assert.False(t, memory.IsIncompleteScopeError(&memory.TenantMismatchError{}), "the two refusals never pass for each other")
	assert.Contains(t, (&memory.IncompleteScopeError{Field: "store"}).Error(), "store")
	assert.False(t, memory.IsExpectedError(errors.New("db down")))
	for code, e := range map[apperror.Code]error{
		apperror.CodeMemoryStoreNotFound:      &memory.StoreNotFoundError{},
		apperror.CodeMemoryStoreInvalid:       &memory.InvalidStoreError{Reason: memory.StoreNameRequired},
		apperror.CodeMemoryStoreInUse:         &memory.StoreInUseError{},
		apperror.CodeMemoryStoreAlreadyExists: &memory.StoreAlreadyExistsError{Name: "People"},
		apperror.CodeMemoryNotFound:           &memory.NotFoundError{},
	} {
		wrapped := fmt.Errorf("wrapped: %w", e)
		assert.True(t, memory.IsExpectedError(wrapped), code)
		assert.Equal(t, code, codeOf(wrapped))
	}
}

func TestNewStore(t *testing.T) {
	org, prj, now := uuid.New(), uuid.New(), time.Date(2026, 11, 14, 8, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	s, err := memory.NewStore(org, prj, memory.StoreInput{Name: "  People  ", Backend: memory.BackendNative}, nativeOn, now)
	require.NoError(t, err)
	assert.False(t, s.RememberAPIUsers, "named API users are off unless the store opts in")
	opted, err := memory.NewStore(org, prj, memory.StoreInput{Name: "API", Backend: memory.BackendNative, RememberAPIUsers: true}, nativeOn, now)
	require.NoError(t, err)
	assert.True(t, opted.RememberAPIUsers)
	assert.Equal(t, "People", s.Name)
	assert.Equal(t, memory.BackendNative, s.Backend)
	assert.Equal(t, org, s.OrgID)
	assert.Equal(t, prj, s.ProjectID)
	assert.Len(t, s.PublicID, 21)
	assert.NotEqual(t, uuid.Nil, s.ID)
	assert.Equal(t, now.UTC(), s.CreatedAt)
	assert.Equal(t, now.UTC(), s.UpdatedAt)
}

func TestNewStore_RefusesBadInput(t *testing.T) {
	for name, tc := range map[string]struct {
		in      memory.StoreInput
		enabled []memory.Backend
		want    memory.StoreReason
	}{
		"blank name":      {memory.StoreInput{Name: "  ", Backend: memory.BackendNative}, nativeOn, memory.StoreNameRequired},
		"long name":       {memory.StoreInput{Name: strings.Repeat("é", memory.MaxStoreNameLen+1), Backend: memory.BackendNative}, nativeOn, memory.StoreNameTooLong},
		"no backend on":   {memory.StoreInput{Name: "People", Backend: memory.BackendNative}, nil, memory.StoreBackendDisabled},
		"unknown backend": {memory.StoreInput{Name: "People", Backend: "mem0"}, nativeOn, memory.StoreBackendDisabled},
		"another backend": {memory.StoreInput{Name: "People", Backend: memory.BackendNative}, []memory.Backend{"mem0"}, memory.StoreBackendDisabled},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := memory.NewStore(uuid.New(), uuid.New(), tc.in, tc.enabled, time.Now())
			var inv *memory.InvalidStoreError
			require.ErrorAs(t, err, &inv)
			assert.Equal(t, tc.want, inv.Reason)
			ae, ok := apperror.AsAppError(err)
			require.True(t, ok)
			assert.Equal(t, apperror.CodeMemoryStoreInvalid, ae.Code())
			assert.Equal(t, string(tc.want), ae.Meta()["reason"])
			assert.Equal(t, strconv.Itoa(memory.MaxStoreNameLen), ae.Meta()["max"])
		})
	}
	_, err := memory.NewStore(uuid.New(), uuid.New(), memory.StoreInput{Name: strings.Repeat("é", memory.MaxStoreNameLen), Backend: memory.BackendNative}, nativeOn, time.Now())
	assert.NoError(t, err, "the cap counts characters, not bytes")
}

func TestStoreAlreadyExists_EchoesTheName(t *testing.T) {
	ae, ok := apperror.AsAppError(&memory.StoreAlreadyExistsError{Name: "People"})
	require.True(t, ok)
	assert.Equal(t, "People", ae.Meta()["name"])
}
