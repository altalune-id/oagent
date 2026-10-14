package nats

import (
	"context"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"altalune.id/oagent/internal/kernel/queue"
)

const (
	streamWork      = "WORK"
	streamDLQ       = "DLQ"
	streamBroadcast = "BROADCAST"
)

const (
	dlqSubjectPrefix = "dlq."
	allSubjects      = ">"
)

func dlqSubject(subject string) string { return dlqSubjectPrefix + subject }

func durableName(subject string) string {
	return strings.ReplaceAll(strings.TrimPrefix(subject, queue.JobSubjectPrefix), ".", "_")
}

func streamConfigs() []jetstream.StreamConfig {
	return []jetstream.StreamConfig{
		{
			Name:       streamWork,
			Subjects:   []string{queue.JobSubjectPrefix + allSubjects},
			Retention:  jetstream.WorkQueuePolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   streamReplicas,
			MaxAge:     workMaxAge,
			MaxBytes:   streamMaxBytes,
			Discard:    jetstream.DiscardNew,
			Duplicates: duplicateWindow,
		},
		{
			Name:       streamDLQ,
			Subjects:   []string{dlqSubjectPrefix + queue.JobSubjectPrefix + allSubjects},
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   streamReplicas,
			MaxAge:     dlqMaxAge,
			MaxBytes:   streamMaxBytes,
			Discard:    jetstream.DiscardOld,
			Duplicates: duplicateWindow,
		},
		{
			Name:       streamBroadcast,
			Subjects:   []string{queue.BroadcastSubjectPrefix + allSubjects},
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   streamReplicas,
			MaxAge:     broadcastMaxAge,
			MaxBytes:   broadcastMaxBytes,
			Discard:    jetstream.DiscardOld,
			Duplicates: duplicateWindow,
		},
	}
}

func consumerConfig(j queue.Job) jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Durable:       durableName(j.Subject()),
		FilterSubject: j.Subject(),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       AckWait,
		MaxDeliver:    consumerMaxDeliver,
		MaxAckPending: consumerMaxAckPending,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	}
}

func ensureStreams(ctx context.Context, js jetstream.JetStream) error {
	for _, cfg := range streamConfigs() {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("queue: ensure stream %s: %w", cfg.Name, err)
		}
	}
	return nil
}
