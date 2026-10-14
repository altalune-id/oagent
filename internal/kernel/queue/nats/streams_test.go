package nats

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/queue"
)

func TestJob_Names(t *testing.T) {
	j := queue.Job{Name: "todo.log_completion", Version: 1}
	subject := j.Subject()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"subject", subject, "jobs.todo.log_completion.v1"},
		{"durable", durableName(subject), "todo_log_completion_v1"},
		{"dlq subject", dlqSubject(subject), "dlq.jobs.todo.log_completion.v1"},
		{"broadcast subject", queue.Broadcast{Name: "system.onboarding_completed", Version: 2}.Subject(), "broadcast.system.onboarding_completed.v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}
