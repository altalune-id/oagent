package queue

import (
	"regexp"
	"strconv"
)

// Subject prefixes every driver uses for jobs and broadcasts.
const (
	JobSubjectPrefix       = "jobs."
	BroadcastSubjectPrefix = "broadcast."
)

const namePattern = `^[a-z0-9_]+(\.[a-z0-9_]+)+$`

func subjectFor(prefix, name string, version int) string {
	return prefix + name + ".v" + strconv.Itoa(version)
}

func nameProblem(name string, version int) string {
	if ok, err := regexp.MatchString(namePattern, name); err != nil || !ok {
		return "name must match " + namePattern
	}
	if version < 1 {
		return "version must be at least 1"
	}
	return ""
}
