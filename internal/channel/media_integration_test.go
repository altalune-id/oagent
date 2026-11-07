//go:build integration

package channel_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/channel/channeltest"
	"altalune.id/oagent/internal/run"
)

var _ run.Media = channel.Media{}

func newMediaWorld(t *testing.T) (*pgWorld, tenantScope, *channel.Connection, *channeltest.Fake) {
	t.Helper()
	w := newPgWorld(t)
	s := w.seed(t)
	conn, err := create(in(s), w.svc, s.project, s.agent, input(), "whsec_primary", "owk_device")
	require.NoError(t, err)
	return w, s, conn, &channeltest.Fake{Media: map[string][]byte{"msg_1": []byte("jpeg-bytes")}}
}

func TestMedia_FetchAndMarkReadThroughTheConnection(t *testing.T) {
	w, s, conn, fake := newMediaWorld(t)
	m := channel.Media{Service: w.svc, Provider: fake}
	data, mime, err := m.Fetch(in(s), conn.ID, "msg_1", "image/jpeg", 5<<20)
	require.NoError(t, err)
	assert.Equal(t, []byte("jpeg-bytes"), data)
	assert.Equal(t, "image/jpeg", mime)
	require.NoError(t, m.MarkRead(in(s), conn.ID, "msg_1"))
	assert.Equal(t, []string{"msg_1"}, fake.Read())
}

type uncapped struct {
	*channeltest.Fake
	body []byte
}

func (u uncapped) FetchMedia(
	context.Context, channel.Connection, channel.Secrets, channel.MediaRef, int64,
) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(u.body)), "image/jpeg", nil
}

func TestMedia_OverTheCapRefused(t *testing.T) {
	w, s, conn, fake := newMediaWorld(t)
	p := uncapped{Fake: fake, body: bytes.Repeat([]byte{1}, 11)}
	_, _, err := channel.Media{Service: w.svc, Provider: p}.Fetch(in(s), conn.ID, "msg_2", "image/jpeg", 10)
	assert.True(t, channel.IsTooLargeError(err), "%v", err)
}

func TestMedia_DisabledOrUnknownConnectionReadsNothing(t *testing.T) {
	w, s, conn, fake := newMediaWorld(t)
	m := channel.Media{Service: w.svc, Provider: fake}
	_, _, err := m.Fetch(in(s), uuid.New(), "msg_1", "image/jpeg", 5<<20)
	assert.True(t, channel.IsNotFoundError(err), "%v", err)

	settings := input()
	settings.Status = channel.StatusDisabled
	_, err = w.svc.Update(in(s), conn.ID, settings, nil)
	require.NoError(t, err)
	_, _, err = m.Fetch(in(s), conn.ID, "msg_1", "image/jpeg", 5<<20)
	assert.True(t, channel.IsNotFoundError(err), "%v", err)
	assert.True(t, channel.IsNotFoundError(m.MarkRead(in(s), conn.ID, "msg_1")))
	assert.Empty(t, fake.Read())
}
