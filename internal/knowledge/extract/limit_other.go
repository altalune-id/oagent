//go:build !linux

package extract

// NOTE: the hard memory limit is Linux-only; deploys run on Linux, elsewhere only GOMEMLIMIT applies.
func limitMemory(int, int64) {}

func limitSelf() {}
