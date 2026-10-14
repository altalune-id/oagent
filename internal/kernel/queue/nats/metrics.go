package nats

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type metrics struct {
	published        metric.Int64Counter
	publishFailed    metric.Int64Counter
	emitted          metric.Int64Counter
	emitFailed       metric.Int64Counter
	consumed         metric.Int64Counter
	dlqPublishFailed metric.Int64Counter
	broadcastFailed  metric.Int64Counter
	deliveryAttempts metric.Int64Histogram
}

func newMetrics(meter metric.Meter) (metrics, error) {
	var m metrics
	counters := []struct {
		dst  *metric.Int64Counter
		name string
		desc string
	}{
		{&m.published, "queue.published", "Jobs the WORK stream acknowledged."},
		{&m.publishFailed, "queue.publish_failed", "Jobs Submit could not publish."},
		{&m.emitted, "queue.emitted", "Broadcasts the BROADCAST stream acknowledged."},
		{&m.emitFailed, "queue.emit_failed", "Broadcasts Emit could not publish."},
		{&m.consumed, "queue.consumed", "Job deliveries the consumer settled, by outcome."},
		{&m.dlqPublishFailed, "queue.dlq_publish_failed", "Dead letters the DLQ stream did not acknowledge."},
		{&m.broadcastFailed, "queue.broadcast_failed", "Broadcasts a listener failed to handle."},
	}
	for _, ct := range counters {
		counter, err := meter.Int64Counter(ct.name, metric.WithDescription(ct.desc))
		if err != nil {
			return metrics{}, fmt.Errorf("queue: counter %s: %w", ct.name, err)
		}
		*ct.dst = counter
	}
	h, err := meter.Int64Histogram("queue.delivery_attempts", metric.WithDescription("Delivery attempt number of each consumed job."))
	if err != nil {
		return metrics{}, fmt.Errorf("queue: histogram queue.delivery_attempts: %w", err)
	}
	m.deliveryAttempts = h
	return m, nil
}
