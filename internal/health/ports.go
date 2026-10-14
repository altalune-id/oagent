package health

// Check reports whether one dependency is ready to serve traffic; *pg.HealthMonitor implements it.
type Check interface {
	Ready() bool
}
