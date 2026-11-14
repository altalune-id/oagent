package run

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
)

const (
	oggHeaderLen  = 27
	oggContinued  = 0x01
	oggBOS        = 0x02
	opusHeadLen   = 19
	opusRate      = 48_000
	oggNoGranule  = -1
	oggMaxSeconds = 1 << 32
)

var (
	errOggCapture   = errors.New("run: not an ogg page")
	errOggTruncated = errors.New("run: ogg page runs past the data")
	errOggOpusHead  = errors.New("run: first ogg stream is not opus")
	errOggStream    = errors.New("run: ogg file holds more than one logical stream")
	errOggPacket    = errors.New("run: opus packet has no frame count")
	errOggGranule   = errors.New("run: ogg stream has no usable length")
)

// RFC 6716 Section 3.1: frame length in 48 kHz samples by TOC config.
func opusFrameSamples(config byte) int64 {
	switch {
	case config < 12:
		return [4]int64{480, 960, 1920, 2880}[config%4]
	case config < 16:
		return [2]int64{480, 960}[config%2]
	default:
		return [4]int64{120, 240, 480, 960}[config%4]
	}
}

type opusPackets struct {
	n       int
	head    []byte
	samples int64
	open    bool
}

func (o *opusPackets) segment(seg []byte, last bool) error {
	if len(o.head) < 2 {
		o.head = append(o.head, seg[:min(len(seg), 2-len(o.head))]...)
	}
	o.open = !last
	if o.open {
		return nil
	}
	defer func() { o.n, o.head = o.n+1, o.head[:0] }()
	if o.n < 2 || len(o.head) == 0 {
		return nil
	}
	toc := o.head[0]
	frames := int64(1)
	switch toc & 3 {
	case 1, 2:
		frames = 2
	case 3:
		if len(o.head) < 2 {
			return errOggPacket
		}
		frames = int64(o.head[1] & 0x3f)
	}
	o.samples += frames * opusFrameSamples(toc>>3)
	return nil
}

func oggDuration(data []byte) (time.Duration, error) {
	var (
		serial  uint32
		preSkip int64
		last    int64 = oggNoGranule
		packets opusPackets
	)
	for pos, first := 0, true; pos < len(data); first = false {
		if len(data)-pos < oggHeaderLen {
			return 0, errOggTruncated
		}
		h := data[pos : pos+oggHeaderLen]
		if !bytes.Equal(h[:4], []byte("OggS")) || h[4] != 0 {
			return 0, errOggCapture
		}
		segments := int(h[26])
		bodyStart := pos + oggHeaderLen + segments
		if bodyStart > len(data) {
			return 0, errOggTruncated
		}
		lacing := data[pos+oggHeaderLen : bodyStart]
		bodyLen := 0
		for _, s := range lacing {
			bodyLen += int(s)
		}
		if bodyLen > len(data)-bodyStart {
			return 0, errOggTruncated
		}
		body := data[bodyStart : bodyStart+bodyLen]
		flags := h[5]
		granule := int64(binary.LittleEndian.Uint64(h[6:14])) //nolint:gosec // G115: the granule position is a signed two's-complement field
		pageSerial := binary.LittleEndian.Uint32(h[14:18])
		pos = bodyStart + bodyLen

		if first {
			if flags&oggBOS == 0 || len(body) < opusHeadLen || !bytes.Equal(body[:8], []byte("OpusHead")) {
				return 0, errOggOpusHead
			}
			serial = pageSerial
			preSkip = int64(binary.LittleEndian.Uint16(body[10:12]))
		} else if pageSerial != serial || flags&oggBOS != 0 {
			return 0, errOggStream
		}
		if packets.open && flags&oggContinued == 0 {
			packets.open, packets.head, packets.n = false, packets.head[:0], packets.n+1
		}
		off := 0
		for _, s := range lacing {
			if err := packets.segment(body[off:off+int(s)], s < 255); err != nil {
				return 0, err
			}
			off += int(s)
		}
		if !first && granule != oggNoGranule {
			last = granule
		}
	}
	samples := max(last-preSkip, packets.samples-preSkip)
	if last < preSkip && packets.samples <= preSkip {
		return 0, errOggGranule
	}
	if samples/opusRate >= oggMaxSeconds {
		return 0, errOggGranule
	}
	return time.Duration(samples/opusRate)*time.Second + time.Duration(samples%opusRate)*time.Second/opusRate, nil
}
