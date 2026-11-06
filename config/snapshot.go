package config

// SnapshotDriver selects where the runtime's thread snapshots are kept.
type SnapshotDriver string

// SnapshotDriverPostgres keeps snapshots in the thread_snapshots table.
const SnapshotDriverPostgres SnapshotDriver = "postgres"

// SnapshotConfig picks the snapshot store's driver.
type SnapshotConfig struct {
	Driver SnapshotDriver `yaml:"driver" mapstructure:"driver" validate:"oneof=postgres"`
}
