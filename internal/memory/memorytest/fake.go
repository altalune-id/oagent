package memorytest

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/memory"
)

// Fake is an in-memory memory.Facts that keeps the contract, the org boundary included.
type Fake struct {
	mu       sync.Mutex
	seq      int
	facts    []fakeFact
	extracts bool
}

// Option changes a Fake.
type Option func(*Fake)

// Extracting makes the Fake a driver that extracts facts itself: Record turns each user text into one fact.
func Extracting() Option { return func(f *Fake) { f.extracts = true } }

type fakeFact struct {
	memory.Fact
	scope memory.Scope
	hash  []byte
	seq   int
}

var _ memory.Facts = (*Fake)(nil)

// NewFake returns an empty Fake.
func NewFake(opts ...Option) *Fake {
	f := &Fake{}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Facts implements Backend.
func (f *Fake) Facts() memory.Facts { return f }

// Matrix implements Backend.
func (f *Fake) Matrix(*testing.T) []Cell {
	out := make([]Cell, 0, 32)
	for range 2 {
		org := uuid.New()
		ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org})
		for range 2 {
			prj := uuid.New()
			agents, users := []uuid.UUID{uuid.New(), uuid.New()}, []uuid.UUID{uuid.New(), uuid.New()}
			for range 2 {
				store := uuid.New()
				for _, a := range agents {
					for _, u := range users {
						out = append(out, Cell{Ctx: ctx, Scope: memory.Scope{OrgID: org, ProjectID: prj, StoreID: store, AgentID: a, EndUserID: u}})
					}
				}
			}
		}
	}
	return out
}

// Run implements Backend.
func (f *Fake) Run(*testing.T, Cell) uuid.UUID { return uuid.New() }

// Extracts implements memory.Facts.
func (f *Fake) Extracts() bool { return f.extracts }

func visible(ctx context.Context, s memory.Scope) bool {
	tc, err := tenant.From(ctx)
	return err == nil && tc.OrgID == s.OrgID
}

func (ff fakeFact) in(s memory.Scope) bool {
	return ff.scope.OrgID == s.OrgID && ff.scope.ProjectID == s.ProjectID && ff.scope.StoreID == s.StoreID &&
		ff.scope.EndUserID == s.EndUserID &&
		(s.AgentID == uuid.Nil || ff.scope.AgentID == s.AgentID)
}

func (f *Fake) newestFirst(s memory.Scope) []fakeFact {
	var out []fakeFact
	for _, ff := range f.facts {
		if ff.in(s) {
			out = append(out, ff)
		}
	}
	slices.SortFunc(out, func(a, b fakeFact) int { return b.seq - a.seq })
	return out
}

// Recall implements memory.Facts.
func (f *Fake) Recall(ctx context.Context, s memory.Scope, query string, k int) ([]memory.Fact, error) {
	if err := s.Check(true); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !visible(ctx, s) {
		return nil, nil
	}
	words := strings.Fields(strings.ToLower(query))
	score := func(ff fakeFact) int {
		n := 0
		for w := range strings.FieldsSeq(strings.ToLower(ff.Text)) {
			if slices.Contains(words, strings.Trim(w, ".,!?")) {
				n++
			}
		}
		return n
	}
	all := f.newestFirst(s)
	slices.SortStableFunc(all, func(a, b fakeFact) int { return score(b) - score(a) })
	out := make([]memory.Fact, 0, k)
	for _, ff := range all[:min(k, len(all))] {
		out = append(out, ff.Fact)
	}
	return out, nil
}

// Record implements memory.Facts.
func (f *Fake) Record(ctx context.Context, s memory.Scope, turn memory.Turn) error {
	if err := s.Check(true); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !visible(ctx, s) {
		return &memory.TenantMismatchError{}
	}
	in := turn.Facts
	if f.extracts {
		in = turn.UserTexts
	}
	f.add(s, turn.RunID, memory.CleanFacts(in))
	return nil
}

func (f *Fake) add(s memory.Scope, run uuid.UUID, texts []string) {
	for _, text := range texts {
		h := memory.Hash(text)
		if i := slices.IndexFunc(f.facts, func(ff fakeFact) bool { return ff.in(s) && bytes.Equal(ff.hash, h) }); i >= 0 {
			f.seq++
			f.facts[i].seq, f.facts[i].CreatedAt, f.facts[i].SourceRunID = f.seq, time.Now(), run
			continue
		}
		f.seq++
		f.facts = append(f.facts, fakeFact{
			Fact:  memory.Fact{ID: uuid.NewString(), Text: text, CreatedAt: time.Now(), AgentID: s.AgentID, SourceRunID: run},
			scope: s, hash: h, seq: f.seq,
		})
	}
	if kept := f.newestFirst(s); len(kept) > memory.MaxPerPerson {
		drop := kept[memory.MaxPerPerson:]
		f.facts = slices.DeleteFunc(f.facts, func(ff fakeFact) bool {
			return slices.ContainsFunc(drop, func(d fakeFact) bool { return d.ID == ff.ID })
		})
	}
}

// List implements memory.Facts.
func (f *Fake) List(ctx context.Context, s memory.Scope, cursor string, limit int) (page []memory.Fact, next string, total int, err error) {
	if err := s.Check(false); err != nil {
		return nil, "", 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !visible(ctx, s) {
		return nil, "", 0, nil
	}
	all := f.newestFirst(s)
	after, _ := strconv.Atoi(cursor)
	for _, ff := range all {
		if cursor != "" && ff.seq >= after {
			continue
		}
		if len(page) == limit {
			next = strconv.Itoa(f.seqOf(page[len(page)-1].ID))
			break
		}
		page = append(page, ff.Fact)
	}
	return page, next, len(all), nil
}

func (f *Fake) seqOf(id string) int {
	for _, ff := range f.facts {
		if ff.ID == id {
			return ff.seq
		}
	}
	return 0
}

// Delete implements memory.Facts.
func (f *Fake) Delete(ctx context.Context, s memory.Scope, id string) error {
	if err := s.Check(false); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.facts, func(ff fakeFact) bool { return ff.ID == id && ff.in(s) })
	if i < 0 || !visible(ctx, s) {
		return &memory.NotFoundError{ID: id}
	}
	f.facts = slices.Delete(f.facts, i, i+1)
	return nil
}

// Clear implements memory.Facts.
func (f *Fake) Clear(ctx context.Context, s memory.Scope) (int, error) {
	if err := s.Check(false); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !visible(ctx, s) {
		return 0, nil
	}
	n := len(f.facts)
	f.facts = slices.DeleteFunc(f.facts, func(ff fakeFact) bool { return ff.in(s) })
	return n - len(f.facts), nil
}

// ClearStore implements memory.Facts.
func (f *Fake) ClearStore(ctx context.Context, s memory.Scope) (int, error) {
	if err := s.CheckStore(); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !visible(ctx, s) {
		return 0, nil
	}
	n := len(f.facts)
	f.facts = slices.DeleteFunc(f.facts, func(ff fakeFact) bool {
		return ff.scope.OrgID == s.OrgID && ff.scope.ProjectID == s.ProjectID && ff.scope.StoreID == s.StoreID
	})
	return n - len(f.facts), nil
}

// Seed implements Backend.
func (f *Fake) Seed(_ *testing.T, cell Cell, texts ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(cell.Scope, uuid.Nil, texts)
}
