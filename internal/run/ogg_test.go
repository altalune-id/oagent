package run

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oggPageOf(serial uint32, granule int64, flags byte, packets ...[]byte) []byte {
	var segs, body []byte
	for i, p := range packets {
		n := len(p)
		for n >= 255 {
			segs = append(segs, 255)
			n -= 255
		}
		if i < len(packets)-1 || flags&0x80 == 0 {
			segs = append(segs, byte(n))
		}
		body = append(body, p...)
	}
	h := make([]byte, oggHeaderLen)
	copy(h, "OggS")
	h[5] = flags &^ 0x80
	binary.LittleEndian.PutUint64(h[6:14], uint64(granule))
	binary.LittleEndian.PutUint32(h[14:18], serial)
	h[26] = byte(len(segs))
	return append(append(h, segs...), body...)
}

func oggPage(serial uint32, granule int64, body []byte) []byte {
	return oggPageOf(serial, granule, 0, body)
}

func bos(page []byte) []byte {
	page[5] |= oggBOS
	return page
}

func opusHead(preSkip uint16) []byte {
	b := make([]byte, opusHeadLen)
	copy(b, "OpusHead")
	b[8], b[9] = 1, 1
	binary.LittleEndian.PutUint16(b[10:12], preSkip)
	binary.LittleEndian.PutUint32(b[12:16], opusRate)
	return b
}

func oggFile(pages ...[]byte) []byte {
	return bytes.Join(pages, nil)
}

func head(serial uint32, preSkip uint16) []byte {
	return bos(oggPage(serial, 0, opusHead(preSkip)))
}

func threeSeconds() []byte {
	return oggFile(
		head(7, 312),
		oggPage(7, 0, []byte("OpusTags")),
		oggPage(7, 312+opusRate, make([]byte, 300)),
		oggPage(7, 312+3*opusRate, []byte{1, 2, 3}),
	)
}

func celtSecond() []byte { return []byte{0xFB, 50} }

func packets(n int, p []byte) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = p
	}
	return out
}

func longPayload(t *testing.T, n int) []byte {
	t.Helper()
	pages := [][]byte{head(7, 0), oggPage(7, 0, []byte("OpusTags"))}
	for left := n; left > 0; left -= 200 {
		pages = append(pages, oggPageOf(7, opusRate, 0, packets(min(left, 200), celtSecond())...))
	}
	return oggFile(pages...)
}

func TestOggDuration(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want time.Duration
	}{
		{name: "known duration", data: threeSeconds(), want: 3 * time.Second},
		{name: "sub-second", data: oggFile(head(1, 0), oggPage(1, 0, []byte("OpusTags")), oggPage(1, opusRate*4/10, []byte{1})), want: 400 * time.Millisecond},
		{name: "trailing page without a granule", data: oggFile(threeSeconds(), oggPage(7, oggNoGranule, []byte{1})), want: 3 * time.Second},
		{name: "granule equal to pre-skip", data: oggFile(head(1, 312), oggPage(1, 312, []byte("OpusTags"))), want: 0},
		{name: "a short granule cannot hide a long payload", data: longPayload(t, 10), want: 10 * time.Second},
		{name: "packets alone give the length", data: oggFile(head(1, 0), oggPage(1, oggNoGranule, []byte("OpusTags")), oggPageOf(1, oggNoGranule, 0, celtSecond(), []byte{0x00}, []byte{0x09})), want: time.Second + 10*time.Millisecond + 40*time.Millisecond},
		{name: "a packet laced across pages", data: oggFile(
			head(1, 0),
			oggPageOf(1, oggNoGranule, 0x80, bytes.Repeat([]byte{0xFB}, 510)),
			oggPageOf(1, oggNoGranule, oggContinued, []byte{50, 0x00}, celtSecond()),
		), want: time.Second},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oggDuration(tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOggDuration_TooLongByPayload(t *testing.T) {
	got, err := oggDuration(longPayload(t, 301))
	require.NoError(t, err)
	assert.Greater(t, got, MaxVoiceLength, "301 one-second packets under a one-second granule still read as over 5 min")
}

func TestOggDuration_RealFile(t *testing.T) {
	data, err := os.ReadFile("testdata/voice.ogg")
	require.NoError(t, err)
	got, err := oggDuration(data)
	require.NoError(t, err)
	assert.InDelta(t, 2500*time.Millisecond, got, float64(20*time.Millisecond), "the last packet may run past the trimmed end")
}

func TestOggDuration_Unreadable(t *testing.T) {
	full := threeSeconds()
	overflow := oggPage(7, 312+opusRate, []byte{1, 2, 3})
	overflow[oggHeaderLen] = 200
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{name: "not ogg", data: []byte("ID3\x04 this is an mp3 file, not ogg at all"), want: errOggCapture},
		{name: "empty", data: nil, want: errOggGranule},
		{name: "truncated header", data: full[:len(full)-len(oggPage(7, 0, []byte{1, 2, 3}))+10], want: errOggTruncated},
		{name: "truncated body", data: full[:len(full)-1], want: errOggTruncated},
		{name: "truncated segment table", data: append(head(7, 312), oggPage(7, 1, make([]byte, 600))[:oggHeaderLen+1]...), want: errOggTruncated},
		{name: "page length past the data", data: oggFile(head(7, 312), overflow), want: errOggTruncated},
		{name: "no OpusHead", data: oggFile(bos(oggPage(7, 0, []byte("\x01vorbis-header-bytes"))), oggPage(7, opusRate, []byte{1})), want: errOggOpusHead},
		{name: "short OpusHead", data: oggFile(bos(oggPage(7, 0, []byte("OpusHead"))), oggPage(7, opusRate, []byte{1})), want: errOggOpusHead},
		{name: "first page without BOS", data: oggFile(oggPage(7, 0, opusHead(0)), oggPage(7, opusRate, []byte{1})), want: errOggOpusHead},
		{name: "second logical stream", data: oggFile(threeSeconds(), oggPage(9, 312+60*opusRate, []byte{1})), want: errOggStream},
		{name: "chained stream", data: oggFile(threeSeconds(), head(9, 0), oggPage(9, 60*opusRate, []byte("OpusTags"))), want: errOggStream},
		{name: "second BOS on the same stream", data: oggFile(threeSeconds(), head(7, 0)), want: errOggStream},
		{name: "code 3 without a frame count", data: oggFile(head(7, 0), oggPage(7, 0, []byte("OpusTags")), oggPage(7, opusRate, []byte{0xFB})), want: errOggPacket},
		{name: "granule below pre-skip and no audio", data: oggFile(head(7, 312), oggPage(7, 100, []byte("OpusTags"))), want: errOggGranule},
		{name: "only header pages", data: head(7, 312), want: errOggGranule},
		{name: "only a missing granule", data: oggFile(head(7, 0), oggPage(7, oggNoGranule, []byte("OpusTags"))), want: errOggGranule},
		{name: "implausible granule", data: oggFile(head(7, 0), oggPage(7, 1<<62, []byte{1})), want: errOggGranule},
		{name: "bad version", data: func() []byte { b := threeSeconds(); b[4] = 1; return b }(), want: errOggCapture},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oggDuration(tt.data)
			require.ErrorIs(t, err, tt.want)
			assert.Zero(t, got)
		})
	}
}

func FuzzOggDuration(f *testing.F) {
	f.Add(threeSeconds())
	f.Add(oggFile(threeSeconds(), oggPage(7, oggNoGranule, []byte{1})))
	f.Add(oggFile(threeSeconds(), oggPage(9, 312+60*opusRate, []byte{1})))
	f.Add(oggFile(threeSeconds(), head(9, 0)))
	f.Add(oggFile(head(7, 312), oggPage(7, 100, []byte{1})))
	f.Add(oggFile(bos(oggPage(7, 0, []byte("\x01vorbis"))), oggPage(7, opusRate, []byte{1})))
	f.Add(oggFile(head(7, 0), oggPageOf(7, oggNoGranule, 0x80, bytes.Repeat([]byte{0xFB}, 510)), oggPageOf(7, 0, oggContinued, []byte{50})))
	f.Add(oggFile(head(7, 0), oggPage(7, 0, []byte("OpusTags")), oggPage(7, opusRate, []byte{0xFB})))
	f.Add(threeSeconds()[:40])
	f.Add([]byte("ID3\x04"))
	if data, err := os.ReadFile("testdata/voice.ogg"); err == nil {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := oggDuration(data)
		if err != nil {
			assert.Zero(t, d)
			return
		}
		assert.GreaterOrEqual(t, d, time.Duration(0))
	})
}
