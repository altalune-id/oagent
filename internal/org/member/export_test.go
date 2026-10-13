package member

// SetAfterLock makes s call fn right after it locks the owner rows, so tests can widen the race window.
func SetAfterLock(s *Service, fn func()) { s.afterLock = fn }
