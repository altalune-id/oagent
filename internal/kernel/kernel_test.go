package kernel

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCloser struct {
	name  string
	err   error
	order *[]string
}

func (c fakeCloser) Close() error {
	*c.order = append(*c.order, c.name)
	return c.err
}

func TestKernel_Close_ReverseOrder(t *testing.T) {
	var order []string
	k := &Kernel{}
	k.AddCloser(fakeCloser{name: "a", order: &order})
	k.AddCloser(fakeCloser{name: "b", order: &order})

	require.NoError(t, k.Close())
	assert.Equal(t, []string{"b", "a"}, order)
}

func TestKernel_Close_JoinsErrors(t *testing.T) {
	var order []string
	k := &Kernel{}
	k.AddCloser(fakeCloser{name: "a", err: errors.New("a"), order: &order})
	k.AddCloser(fakeCloser{name: "b", err: errors.New("b"), order: &order})

	err := k.Close()
	assert.ErrorContains(t, err, "a")
	assert.ErrorContains(t, err, "b")
}

func TestKernel_AddCloser_NilSafe(t *testing.T) {
	k := &Kernel{}
	k.AddCloser(nil) // nil closer ignored
	assert.NoError(t, k.Close())

	var nilKernel *Kernel
	nilKernel.AddCloser(fakeCloser{name: "x", order: &[]string{}})
	assert.NoError(t, nilKernel.Close())
}
