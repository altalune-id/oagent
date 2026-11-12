package run

import (
	"context"
	"strings"
)

const (
	approvalChannelWhatsApp = "whatsapp"
	chatAnswerScan          = 50
)

// SECURITY: only the requester decides: a DM's first message, or in a group a quote of the prompt or an unquoted yes/no word.
func (s *Service) decideFromChat(ctx context.Context, t *turn, a *RunApproval) error {
	th := t.thread
	if a.Channel != approvalChannelWhatsApp || !a.Requester.Valid || a.PromptAfterSeq == nil ||
		(th.Kind != threadKindDM && th.Kind != threadKindGroup) {
		return nil
	}
	// SECURITY: only the requester's messages received after the prompt went out may decide.
	msgs, err := s.store.PendingMessages(ctx, th.ID, a.Requester.UUID, *a.PromptAfterSeq, chatAnswerScan)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Role != RoleUser || m.SpeakerID == nil || *m.SpeakerID != a.Requester.UUID {
			continue
		}
		text := strings.TrimSpace(textOf(m))
		approved, matched := s.approvals.MatchWord(text)
		quoted := a.PromptExternalID != "" && m.QuotedID() == a.PromptExternalID
		if th.Kind == threadKindGroup && !quoted && (m.QuotedID() != "" || !matched) {
			continue
		}
		answer := ChatAnswer{ApprovalID: a.ID, By: a.Requester.UUID, Approved: matched && approved, Text: text}
		if answer.Approved {
			answer.Text = ""
		}
		return s.uow.RunInTx(ctx, func(ctx context.Context) error {
			ok, err := s.approvals.DecideFromChat(ctx, answer)
			if err != nil || !ok {
				return err
			}
			return s.store.AnswerMessage(ctx, t.token, th.ID, m.ID, t.run.ID)
		})
	}
	return nil
}
