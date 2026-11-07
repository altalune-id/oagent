package entity

import "github.com/go-jet/jet/v2/postgres"

// IngestDeliveries is the jet binding for the ingest_deliveries table.
type IngestDeliveries struct {
	postgres.Table

	OrgID        postgres.ColumnString
	ProjectID    postgres.ColumnString
	ConnectionID postgres.ColumnString
	DeliveryID   postgres.ColumnString
	ReceivedAt   postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewIngestDeliveries builds the ingest_deliveries binding.
func NewIngestDeliveries(schema, tablePrefix string) *IngestDeliveries {
	if schema == "" {
		schema = "public"
	}
	var (
		orgID        = postgres.StringColumn("org_id")
		projectID    = postgres.StringColumn("project_id")
		connectionID = postgres.StringColumn("connection_id")
		deliveryID   = postgres.StringColumn("delivery_id")
		receivedAt   = postgres.TimestampzColumn("received_at")
		all          = postgres.ColumnList{
			orgID, projectID, connectionID, deliveryID, receivedAt,
		}
	)
	return &IngestDeliveries{
		Table:        postgres.NewTable(schema, tablePrefix+"ingest_deliveries", "ingest_deliveries", all...),
		OrgID:        orgID,
		ProjectID:    projectID,
		ConnectionID: connectionID,
		DeliveryID:   deliveryID,
		ReceivedAt:   receivedAt,
		AllColumns:   all,
	}
}
