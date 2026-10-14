package queue

// Job names one kind of queued work and the version of its payload.
type Job struct {
	Name    string
	Version int
}

// Subject returns the subject the job is published on.
func (j Job) Subject() string { return subjectFor(JobSubjectPrefix, j.Name, j.Version) }

// Validate reports an *InvalidJobError when the name or version breaks the naming rule.
func (j Job) Validate() error {
	if reason := nameProblem(j.Name, j.Version); reason != "" {
		return &InvalidJobError{Job: j, Reason: reason}
	}
	return nil
}

// Broadcast names one kind of notice every running instance reacts to.
type Broadcast struct {
	Name    string
	Version int
}

// Subject returns the subject the broadcast is published on.
func (b Broadcast) Subject() string { return subjectFor(BroadcastSubjectPrefix, b.Name, b.Version) }

// Validate reports an *InvalidBroadcastError when the name or version breaks the naming rule.
func (b Broadcast) Validate() error {
	if reason := nameProblem(b.Name, b.Version); reason != "" {
		return &InvalidBroadcastError{Broadcast: b, Reason: reason}
	}
	return nil
}
