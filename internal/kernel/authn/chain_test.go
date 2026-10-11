package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/session"
)

type fakeAuthn struct {
	p     session.Principal
	err   error
	calls int
}

func (f *fakeAuthn) Authenticate(context.Context, Credential) (session.Principal, error) {
	f.calls++
	return f.p, f.err
}

func TestChain_FirstSuccessWins(t *testing.T) {
	first := &fakeAuthn{err: errors.New("not mine")}
	second := &fakeAuthn{p: session.Principal{Source: session.SourceBasic, Name: "second"}}
	third := &fakeAuthn{p: session.Principal{Source: session.SourceBasic, Name: "third"}}

	p, err := Chain{first, second, third}.Authenticate(context.Background(), Credential{Kind: KindBasic})
	require.NoError(t, err)
	assert.Equal(t, "second", p.Name)
	assert.Equal(t, 0, third.calls)
}

func TestChain_AllFailIsOneOpaqueError(t *testing.T) {
	a := &fakeAuthn{err: errors.New("cause a")}
	b := &fakeAuthn{err: errors.New("cause b")}

	_, err := Chain{a, b}.Authenticate(context.Background(), Credential{Kind: KindAPIKey})
	require.Error(t, err)
	assert.True(t, IsUnauthorizedError(err))
	assert.Equal(t, "authn: unauthorized", err.Error())
	assert.NotContains(t, err.Error(), "cause")

	ue, ok := errors.AsType[*UnauthorizedError](err)
	require.True(t, ok)
	assert.ErrorContains(t, ue.Causes(), "cause a")
	assert.ErrorContains(t, ue.Causes(), "cause b")
	assert.Nil(t, errors.Unwrap(err))
}

func TestChain_Empty(t *testing.T) {
	_, err := Chain{}.Authenticate(context.Background(), Credential{Kind: KindAPIKey})
	assert.True(t, IsUnauthorizedError(err))
}

func TestUnauthorizedError_ToAppError(t *testing.T) {
	ae := (&UnauthorizedError{causes: errors.New("secret detail")}).ToAppError()
	assert.Equal(t, apperror.CodeUnauthenticated, ae.Code())
	assert.Equal(t, "Sign in required", ae.Message())
	assert.NotContains(t, ae.Error(), "secret detail")
}
