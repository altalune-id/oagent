package run

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/runtime"
)

// MemoryScope is whose facts a leg recalls and records: one end user of one agent in one memory store of one project.
type MemoryScope struct {
	OrgID, ProjectID, StoreID, AgentID, EndUserID uuid.UUID
}

// RecalledFact is one fact recalled for a leg. SECURITY: the person's own words; never log it.
type RecalledFact struct {
	Text      string
	CreatedAt time.Time
}

// MemoryRecord is what a leg hands back: facts for a store that does not extract, user texts for one that does.
type MemoryRecord struct {
	RunID     uuid.UUID
	Facts     []string
	UserTexts []string
}

// StoreMode is how a store takes part in a run: whether it extracts facts itself, and whether it remembers named API users.
type StoreMode struct {
	Extracts, APIUsers bool
}

// Memory recalls and records an end user's facts for an agent in one memory store.
type Memory interface {
	// Store reports how scope's store takes part in a run; a gone store or a disabled backend is an error.
	Store(ctx context.Context, scope MemoryScope) (StoreMode, error)
	Recall(ctx context.Context, scope MemoryScope, query string) ([]RecalledFact, error)
	Record(ctx context.Context, scope MemoryScope, rec MemoryRecord) error
}

// Rememberer is how far memory may follow a speaker.
type Rememberer int

// The rememberer classes.
const (
	// RememberNever is an API key acting for itself or an anonymous widget visitor.
	RememberNever Rememberer = iota
	// RememberAlways is a person the server tells apart: a WhatsApp sender, a signed widget user, a member's personal key.
	RememberAlways
	// RememberNamedAPIUser is an Oagent-User an integration names; remembered only on a store that opts in.
	RememberNamedAPIUser
)

// Rememberers tells how far memory may follow an end user.
type Rememberers interface {
	Rememberable(ctx context.Context, endUserID uuid.UUID) (Rememberer, error)
}

// WithMemory lets eligible legs recall and record facts through m, with p deciding who is a person.
func (s *Service) WithMemory(m Memory, p Rememberers) *Service {
	s.memory, s.rememberers = m, p
	return s
}

// WithKeyPrefix is the API key prefix the runtime masks in memory text.
func (s *Service) WithKeyPrefix(prefix string) *Service { s.keyPrefix = prefix; return s }

const recordTimeout = 5 * time.Second

//nolint:gochecknoglobals // a fixed list.
var recordable = []Status{StatusCompleted, StatusLimitReached, StatusAwaitingApproval}

type memoryLeg struct {
	scope MemoryScope
	mode  runtime.ExtractMode
}

type storeMode struct {
	StoreMode
	ok bool
}

// SECURITY: memory never runs for a draft, a group or a Try thread; the leg check narrows this to one rememberable speaker.
func (s *Service) mayRemember(t *turn) bool {
	v := t.version
	return s.memory != nil && v.MemoryStoreID != uuid.Nil && !v.Draft && (t.thread.Kind == threadKindDM || t.thread.Kind == threadKindAPI)
}

// NOTE: fail-closed; a store that is gone or whose backend is off gives the turn no memory.
func (s *Service) storeMode(ctx context.Context, t *turn) (StoreMode, bool) {
	if t.memStore == nil {
		scope := MemoryScope{OrgID: t.run.OrgID, ProjectID: t.run.ProjectID, StoreID: t.version.MemoryStoreID, AgentID: t.version.AgentID}
		m, err := s.memory.Store(ctx, scope)
		if err != nil {
			s.log.WarnContext(ctx, "run: memory store unavailable; no memory", "run", t.run.PublicID, "err", err.Error())
		}
		t.memStore = &storeMode{StoreMode: m, ok: err == nil}
	}
	return t.memStore.StoreMode, t.memStore.ok
}

func separateHelper(v *AgentVersion) bool { return v.HelperModel != "" && v.HelperModel != v.Model }

// NOTE: a leg that may extract has helper prices pinned with the run; a priced run never takes on helper tokens it cannot price.
func (s *Service) pinHelper(ctx context.Context, t *turn, p *Prices) *Prices {
	if p == nil || !s.mayRemember(t) {
		return p
	}
	if m, ok := s.storeMode(ctx, t); !ok || m.Extracts {
		return p
	}
	pinned := *p
	src := p
	if separateHelper(t.version) {
		m, err := s.providers.ResolveModel(ctx, t.version.ProviderID, t.version.HelperModel)
		if err != nil || m.Prices == nil {
			s.log.DebugContext(ctx, "run: helper model unresolved or unpriced; the leg recalls only", "run", t.run.PublicID)
			return &pinned
		}
		t.helper, src = &m, m.Prices
	}
	pinned.HelperInput, pinned.HelperOutput = src.Input, src.Output
	pinned.HelperCacheRead, pinned.HelperCacheWrite = src.CacheRead, src.CacheWrite
	return &pinned
}

func (s *Service) resolveHelper(ctx context.Context, t *turn) {
	if t.helper != nil || !s.mayRemember(t) || !separateHelper(t.version) {
		return
	}
	if m, ok := s.storeMode(ctx, t); !ok || m.Extracts {
		return
	}
	if m, err := s.providers.ResolveModel(ctx, t.version.ProviderID, t.version.HelperModel); err == nil {
		t.helper = &m
	}
}

func extractMode(t *turn, extracts bool) runtime.ExtractMode {
	switch {
	case extracts:
		return runtime.ExtractUserTexts
	case t.run.Prices != nil && t.run.Prices.HelperInput == nil:
		return runtime.ExtractNone
	case separateHelper(t.version) && t.helper == nil:
		return runtime.ExtractNone
	default:
		return runtime.ExtractFacts
	}
}

// SECURITY: the scope is the single speaker of the leg's own messages, never the thread; an API thread's speaker must be its actor.
func (s *Service) memoryLeg(ctx context.Context, t *turn) *memoryLeg {
	if !s.mayRemember(t) {
		return nil
	}
	var speaker uuid.UUID
	for _, m := range t.taken {
		if m.Role != RoleUser {
			continue
		}
		if m.SpeakerID == nil {
			return nil
		}
		if speaker != uuid.Nil && speaker != *m.SpeakerID {
			return nil
		}
		speaker = *m.SpeakerID
	}
	if speaker == uuid.Nil {
		return nil
	}
	if t.thread.Kind == threadKindAPI && speaker != t.thread.ActorEndUserID {
		s.log.WarnContext(ctx, "run: the leg's speaker is not the thread's actor; no memory", "run", t.run.PublicID)
		return nil
	}
	class, err := s.rememberers.Rememberable(ctx, speaker)
	if err != nil {
		s.log.WarnContext(ctx, "run: end user lookup failed; no memory", "run", t.run.PublicID, "err", err.Error())
		return nil
	}
	if class == RememberNever {
		return nil
	}
	mode, usable := s.storeMode(ctx, t)
	if !usable {
		return nil
	}
	// SECURITY: any key of the project can name an Oagent-User, so a named API user is remembered only on a store that opts in.
	if class == RememberNamedAPIUser && !mode.APIUsers {
		return nil
	}
	scope := MemoryScope{
		OrgID: t.run.OrgID, ProjectID: t.run.ProjectID, StoreID: t.version.MemoryStoreID, AgentID: t.version.AgentID, EndUserID: speaker,
	}
	return &memoryLeg{scope: scope, mode: extractMode(t, mode.Extracts)}
}

// NOTE: fail-open; a failed recall sends no facts and the turn still runs.
func (s *Service) remember(ctx context.Context, t *turn, req *runtime.TurnRequest) {
	leg := s.memoryLeg(ctx, t)
	if leg == nil {
		return
	}
	text := turnText(t.taken)
	query := s.conversation(ctx, t, text)
	if query == "" {
		query = lastRunes(text, maxKnowledgeQueryRunes)
	}
	facts, err := s.memory.Recall(ctx, leg.scope, query)
	if err != nil {
		s.log.WarnContext(ctx, "run: memory recall failed; the leg runs without facts", "run", t.run.PublicID, "err", err.Error())
		facts = nil
	}
	sent := make([]runtime.MemoryFact, 0, runtime.MaxMemoryFacts)
	for _, f := range facts {
		text := strings.TrimSpace(f.Text)
		if utf8.RuneCountInString(text) > runtime.MaxMemoryFactRunes {
			text = string([]rune(text)[:runtime.MaxMemoryFactRunes])
		}
		if text != "" && len(sent) < runtime.MaxMemoryFacts {
			sent = append(sent, runtime.MemoryFact{Text: text, CreatedAt: f.CreatedAt.UTC()})
		}
	}
	req.Memory = runtime.NewMemoryRequest(sent, leg.mode)
	req.Guardrails = &runtime.Guardrails{KeyPrefix: s.keyPrefix}
	if leg.mode == runtime.ExtractFacts && t.helper != nil {
		h := t.helper
		req.Agent.HelperModel = &runtime.ModelSpec{Provider: h.Provider, BaseURL: h.BaseURL, APIKey: h.APIKey, ModelID: t.version.HelperModel}
		if b := req.Limits.Budget; b != nil {
			p := t.run.Prices
			b.HelperPrices = &runtime.Prices{
				Input: orZero(p.HelperInput), Output: orZero(p.HelperOutput),
				CacheRead: orZero(p.HelperCacheRead), CacheWrite: orZero(p.HelperCacheWrite),
			}
		}
	}
	t.memory = leg
}

func orZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// SECURITY: runs only after the settle committed, on its own context, never in the terminal transaction; a failure is logged and counted.
func (s *Service) record(ctx context.Context, t *turn, term *runtime.Terminal) {
	leg := t.memory
	if leg == nil || term.Memory == nil || !slices.Contains(recordable, t.run.Status) {
		return
	}
	// SECURITY: the runtime's memory output is cut to the caps again here, the trust boundary.
	rec := MemoryRecord{RunID: t.run.ID}
	m := term.Memory
	switch {
	case len(m.Facts) > 0 && len(m.UserTexts) > 0,
		len(m.Facts) > 0 && leg.mode != runtime.ExtractFacts,
		len(m.UserTexts) > 0 && leg.mode != runtime.ExtractUserTexts:
		s.log.WarnContext(ctx, "run: memory does not match the mode sent; nothing recorded", "run", t.run.PublicID)
		return
	case len(m.Facts) > 0:
		rec.Facts = runtime.Clean(m.Facts, runtime.MaxMemoryFactRunes, runtime.MaxExtractedFacts)
	case len(m.UserTexts) > 0:
		rec.UserTexts = runtime.Clean(m.UserTexts, runtime.MaxUserTextRunes, runtime.MaxUserTexts)
	}
	if len(rec.Facts) == 0 && len(rec.UserTexts) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := s.memory.Record(ctx, leg.scope, rec); err != nil {
		if s.recordErr != nil {
			s.recordErr.Add(ctx, 1)
		}
		s.log.WarnContext(ctx, "run: memory record failed", "run", t.run.PublicID, "count", len(rec.Facts)+len(rec.UserTexts), "err", err.Error())
	}
}
