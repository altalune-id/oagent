// Package health answers the liveness and readiness probes.
package health

// Service combines the readiness checks of the process's dependencies.
type Service struct {
	checks []Check
}

// NewService builds a Service over checks; nil checks are skipped.
func NewService(checks ...Check) *Service {
	s := &Service{}
	for _, c := range checks {
		if c != nil {
			s.checks = append(s.checks, c)
		}
	}
	return s
}

// Ready reports whether every check is ready; with no checks it is always ready.
func (s *Service) Ready() bool {
	for _, c := range s.checks {
		if !c.Ready() {
			return false
		}
	}
	return true
}
