package session

// SetBatch lowers the DeleteExpired batch size so tests can cross a batch boundary.
func SetBatch(s *PostgresStore, n int) { s.batch = n }
