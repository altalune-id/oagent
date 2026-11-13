package extract

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const memoryHeadroom = 256 << 20

func limitMemory(pid int, limit int64) {
	if limit <= 0 {
		return
	}
	bound := uint64(limit) + memoryHeadroom
	_ = unix.Prlimit(pid, unix.RLIMIT_DATA, &unix.Rlimit{Cur: bound, Max: bound}, nil)
}

func limitSelf() {
	limit, err := strconv.ParseInt(os.Getenv("GOMEMLIMIT"), 10, 64)
	if err != nil || limit <= 0 {
		return
	}
	bound := uint64(limit) + memoryHeadroom
	_ = unix.Setrlimit(unix.RLIMIT_DATA, &unix.Rlimit{Cur: bound, Max: bound})
}
