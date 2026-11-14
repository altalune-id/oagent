package run

import (
	"math"
	"math/bits"
)

// Prices are the price snapshot of a run in micro-USD per 1M tokens, Transcription per audio minute; a nil class is unpriced.
type Prices struct {
	Input, Output, CacheRead, CacheWrite, Embedding              *int64
	HelperInput, HelperOutput, HelperCacheRead, HelperCacheWrite *int64
	Transcription                                                *int64
}

const (
	tokensPerPrice  = 1_000_000
	secondsPerPrice = 60
)

// Cost is the run's cost in micro-USD: ceil(units x price / per) per class, summed; unpriced units make priced false.
func Cost(u Usage, p *Prices) (micros int64, priced bool) {
	if p == nil {
		return 0, false
	}
	for _, c := range []struct {
		units int64
		price *int64
		per   uint64
	}{
		{u.InputTokens, p.Input, tokensPerPrice}, {u.OutputTokens, p.Output, tokensPerPrice},
		{u.CacheReadTokens, p.CacheRead, tokensPerPrice}, {u.CacheWriteTokens, p.CacheWrite, tokensPerPrice},
		{u.EmbeddingTokens, p.Embedding, tokensPerPrice},
		{u.HelperInputTokens, p.HelperInput, tokensPerPrice}, {u.HelperOutputTokens, p.HelperOutput, tokensPerPrice},
		{u.HelperCacheReadTokens, p.HelperCacheRead, tokensPerPrice}, {u.HelperCacheWriteTokens, p.HelperCacheWrite, tokensPerPrice},
		{u.TranscriptionSeconds, p.Transcription, secondsPerPrice},
	} {
		if c.units == 0 {
			continue
		}
		if c.price == nil {
			return 0, false
		}
		micros = addSat(micros, classCost(c.units, *c.price, c.per))
	}
	return micros, true
}

// Cents is micros rounded up to whole US cents.
func Cents(micros int64) int64 { return ceilDiv(micros, 10_000) }

func classCost(units, price int64, per uint64) int64 {
	hi, lo := bits.Mul64(uint64(max(units, 0)), uint64(max(price, 0)))
	if hi >= per {
		return math.MaxInt64
	}
	q, r := bits.Div64(hi, lo, per)
	if q >= math.MaxInt64 {
		return math.MaxInt64
	}
	if r > 0 {
		q++
	}
	return int64(q)
}

func addSat(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

func ceilDiv(a, b int64) int64 {
	q := a / b
	if a%b > 0 {
		q++
	}
	return q
}
