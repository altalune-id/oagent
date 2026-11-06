package boot

import (
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/run"
)

func consumerOff(o *options, cfg config.Config) string {
	switch {
	case !o.consumer:
		return "WithConsumer(false)"
	case o.schedulerOnly:
		return "WithSchedulerOnly"
	case !cfg.Queue.Enabled:
		return "queue.enabled=false"
	}
	return ""
}

func queueJobs() []queue.Job { return []queue.Job{run.TurnJob} }

func queueHandlers(s *Services) []queue.Handler {
	if s.RunTurns == nil {
		return nil
	}
	return []queue.Handler{s.RunTurns.Handler()}
}

func queueListeners(_ *Services) []queue.Listener { return nil }
