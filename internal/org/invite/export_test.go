package invite

// SetAfterLive makes s call fn right after it reads the live invite for an email, so tests can widen the race window.
func SetAfterLive(s *Service, fn func()) { s.afterLive = fn }
