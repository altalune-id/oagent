package run

import (
	"math"
	"math/bits"
)

// Prices are the price snapshot of a run in micro-USD per 1M tokens; a nil class is unpriced.
type Prices struct {
	Input, Output, CacheRead, CacheWrite *int64
}

const tokensPerPrice = 1_000_000

// Cost is the run's cost in micro-USD: ceil(tokens x price / 1 000 000) per class, summed; unpriced tokens make priced false.
func Cost(u Usage, p *Prices) (micros int64, priced bool) {
	if p == nil {
		return 0, false
	}
	for _, c := range []struct {
		tokens int64
		price  *int64
	}{{u.InputTokens, p.Input}, {u.OutputTokens, p.Output}, {u.CacheReadTokens, p.CacheRead}, {u.CacheWriteTokens, p.CacheWrite}} {
		if c.tokens == 0 {
			continue
		}
		if c.price == nil {
			return 0, false
		}
		micros = addSat(micros, classCost(c.tokens, *c.price))
	}
	return micros, true
}

// Cents is micros rounded up to whole US cents.
func Cents(micros int64) int64 { return ceilDiv(micros, 10_000) }

func classCost(tokens, price int64) int64 {
	hi, lo := bits.Mul64(uint64(max(tokens, 0)), uint64(max(price, 0)))
	if hi >= tokensPerPrice {
		return math.MaxInt64
	}
	q, r := bits.Div64(hi, lo, tokensPerPrice)
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
