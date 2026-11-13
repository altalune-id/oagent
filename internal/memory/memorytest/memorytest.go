// Package memorytest is the contract every memory.Facts driver passes.
package memorytest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/memory"
)

// Cell is one scope of the matrix with a context bound to its org.
type Cell struct {
	Ctx   context.Context
	Scope memory.Scope
}

// Backend is the driver under test and the rows it needs.
type Backend interface {
	Facts() memory.Facts
	// Matrix returns 2 orgs x 2 projects x 2 stores x 2 agents x 2 end users, each a scope whose rows exist.
	Matrix(t *testing.T) []Cell
	// Run returns a run of cell's project the driver may name as a fact's source.
	Run(t *testing.T, cell Cell) uuid.UUID
	// Seed stores texts as facts of cell directly, each newer than the one before, bypassing Record.
	Seed(t *testing.T, cell Cell, texts ...string)
}

// Run checks the memory.Facts contract; newBackend returns a fresh, empty backend for each case.
func Run(t *testing.T, newBackend func(t *testing.T) Backend) {
	t.Helper()
	probe := newBackend(t)
	t.Run("IncompleteScopesAreRefused", func(t *testing.T) { incomplete(t, probe) })
	t.Run("EachScopeSeesOnlyItsOwnFacts", func(t *testing.T) { isolation(t, newBackend(t)) })
	t.Run("DeleteOutsideTheScopeIsNotFound", func(t *testing.T) { deleteIsolation(t, newBackend(t)) })
	t.Run("DeleteWithoutAnAgentFindsThePersonsFact", func(t *testing.T) { deletePerson(t, newBackend(t)) })
	t.Run("ClearLeavesEveryOtherScope", func(t *testing.T) { clearIsolation(t, newBackend(t)) })
	t.Run("ClearWithoutAnAgentClearsThePerson", func(t *testing.T) { clearPerson(t, newBackend(t)) })
	t.Run("ClearStoreLeavesEveryOtherStore", func(t *testing.T) { clearStore(t, newBackend(t)) })
	t.Run("AScopeNamingAnotherProjectFindsNothing", func(t *testing.T) { mixedProject(t, newBackend(t)) })
	t.Run("AScopeNamingAnotherStoreFindsNothing", func(t *testing.T) { mixedStore(t, newBackend(t)) })
	t.Run("AnotherOrgsScopeFindsNothing", func(t *testing.T) { foreignOrg(t, newBackend(t)) })
	t.Run("OddQueriesRecallTheNewest", func(t *testing.T) { oddQueries(t, newBackend(t)) })
	t.Run("AMessageSharingOneWordRecallsTheFact", func(t *testing.T) { oneWord(t, newBackend(t)) })
	t.Run("RecallPadsWithTheNewest", func(t *testing.T) { pad(t, newBackend(t)) })
	t.Run("ListPagesNewestFirstWithATotal", func(t *testing.T) { list(t, newBackend(t)) })
	t.Run("ACursorOfAnotherScopeShowsNothingOfIt", func(t *testing.T) { foreignCursor(t, newBackend(t)) })
	if probe.Facts().Extracts() {
		t.Run("RecordAcceptsUserTexts", func(t *testing.T) { userTexts(t, newBackend(t)) })
		t.Run("RecordRefusesAnIncompleteScope", func(t *testing.T) { recordIncomplete(t, newBackend(t)) })
		return
	}
	t.Run("RecordKeepsTheCapsOfOneTurn", func(t *testing.T) { caps(t, newBackend(t)) })
	t.Run("ARepeatedFactIsStoredOnce", func(t *testing.T) { dedupe(t, newBackend(t)) })
	t.Run("TheOldestArePrunedPastTheCap", func(t *testing.T) { prune(t, newBackend(t)) })
	t.Run("ARestatedFactIsFreshAgain", func(t *testing.T) { restated(t, newBackend(t)) })
	t.Run("AFactKeepsItsAgentAndSourceRun", func(t *testing.T) { source(t, newBackend(t)) })
}

func incompleteScope(t *testing.T, err error, field string, msgAndArgs ...any) {
	t.Helper()
	var e *memory.IncompleteScopeError
	if assert.ErrorAs(t, err, &e, msgAndArgs...) {
		assert.Equal(t, field, e.Field, msgAndArgs...)
	}
}

func fact(c Cell) string {
	return fmt.Sprintf("fact of %s %s %s %s %s", c.Scope.OrgID, c.Scope.ProjectID, c.Scope.StoreID, c.Scope.AgentID, c.Scope.EndUserID)
}

func record(t *testing.T, b Backend, c Cell, facts ...string) {
	t.Helper()
	require.NoError(t, b.Facts().Record(c.Ctx, c.Scope, memory.Turn{Facts: facts}))
}

func seed(t *testing.T, b Backend) []Cell {
	t.Helper()
	cells := b.Matrix(t)
	require.Len(t, cells, 32)
	for _, c := range cells {
		b.Seed(t, c, fact(c))
	}
	return cells
}

func texts(facts []memory.Fact) []string {
	out := make([]string, len(facts))
	for i, f := range facts {
		out[i] = f.Text
	}
	return out
}

func listAll(t *testing.T, b Backend, c Cell, scope memory.Scope) []memory.Fact {
	t.Helper()
	facts, _, _, err := b.Facts().List(c.Ctx, scope, "", 50)
	require.NoError(t, err)
	return facts
}

func person(s memory.Scope) memory.Scope {
	s.AgentID = uuid.Nil
	return s
}

func isolation(t *testing.T, b Backend) {
	cells := seed(t, b)
	for _, c := range cells {
		for _, q := range []string{"", "fact of"} {
			got, err := b.Facts().Recall(c.Ctx, c.Scope, q, runtime.MaxMemoryFacts)
			require.NoError(t, err)
			assert.Equal(t, []string{fact(c)}, texts(got), "recall %q sees only its own scope", q)
		}
		assert.Equal(t, []string{fact(c)}, texts(listAll(t, b, c, c.Scope)), "list sees only its own scope")
	}
}

func deleteIsolation(t *testing.T, b Backend) {
	cells := seed(t, b)
	victim := cells[0]
	id := listAll(t, b, victim, victim.Scope)[0].ID
	for _, c := range cells[1:] {
		assert.True(t, memory.IsNotFoundError(b.Facts().Delete(c.Ctx, c.Scope, id)), "a fact of another scope is not found")
		if person(c.Scope) != person(victim.Scope) {
			assert.True(t, memory.IsNotFoundError(b.Facts().Delete(c.Ctx, person(c.Scope), id)), "nor through another person's scope")
		}
	}
	assert.Len(t, listAll(t, b, victim, victim.Scope), 1, "and it is still there")
	require.NoError(t, b.Facts().Delete(victim.Ctx, victim.Scope, id))
	assert.Empty(t, listAll(t, b, victim, victim.Scope))
	for _, c := range cells[1:] {
		assert.Len(t, listAll(t, b, c, c.Scope), 1, "deleting one fact leaves the other scopes")
	}
}

func deletePerson(t *testing.T, b Backend) {
	cells := seed(t, b)
	c := cells[0]
	id := listAll(t, b, c, c.Scope)[0].ID
	require.NoError(t, b.Facts().Delete(c.Ctx, person(c.Scope), id), "an admin delete names the person, not the agent")
	assert.Empty(t, listAll(t, b, c, c.Scope))
}

func clearIsolation(t *testing.T, b Backend) {
	cells := seed(t, b)
	n, err := b.Facts().Clear(cells[0].Ctx, cells[0].Scope)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	for _, c := range cells[1:] {
		assert.Len(t, listAll(t, b, c, c.Scope), 1, "clear leaves every other scope")
	}
}

func clearPerson(t *testing.T, b Backend) {
	cells := seed(t, b)
	c := cells[0]
	n, err := b.Facts().Clear(c.Ctx, person(c.Scope))
	require.NoError(t, err)
	assert.Equal(t, 2, n, "the person's facts under both agents of the store")
	left := 0
	for _, o := range cells {
		got := listAll(t, b, o, o.Scope)
		if person(o.Scope) == person(c.Scope) {
			assert.Empty(t, got)
			continue
		}
		left += len(got)
	}
	assert.Equal(t, 30, left, "the person's facts in the project's other store stay")
}

func storeOf(s memory.Scope) memory.Scope {
	return memory.Scope{OrgID: s.OrgID, ProjectID: s.ProjectID, StoreID: s.StoreID}
}

func clearStore(t *testing.T, b Backend) {
	cells := seed(t, b)
	c := cells[0]
	var theirs Cell
	for _, o := range cells {
		if o.Scope.OrgID != c.Scope.OrgID {
			theirs = o
		}
	}
	foreign := storeOf(theirs.Scope)
	n, err := b.Facts().ClearStore(c.Ctx, foreign)
	if err == nil {
		assert.Zero(t, n, "another org's store under this org's context clears nothing")
	}
	mixed := storeOf(c.Scope)
	mixed.ProjectID = theirs.Scope.ProjectID
	n, err = b.Facts().ClearStore(c.Ctx, mixed)
	if err == nil {
		assert.Zero(t, n, "a store named under another project clears nothing")
	}
	n, err = b.Facts().ClearStore(c.Ctx, storeOf(c.Scope))
	require.NoError(t, err)
	assert.Equal(t, 4, n, "both agents' facts about both people of the store")
	left := 0
	for _, o := range cells {
		got := listAll(t, b, o, o.Scope)
		if o.Scope.StoreID == c.Scope.StoreID {
			assert.Empty(t, got)
			continue
		}
		left += len(got)
	}
	assert.Equal(t, 28, left, "every other store keeps its facts")
	noStore := storeOf(c.Scope)
	noStore.StoreID = uuid.Nil
	_, err = b.Facts().ClearStore(c.Ctx, noStore)
	incompleteScope(t, err, "store")
}

func foreignOrg(t *testing.T, b Backend) {
	cells := seed(t, b)
	var mine, theirs Cell
	for _, c := range cells {
		if c.Scope.OrgID == cells[0].Scope.OrgID {
			mine = c
		} else {
			theirs = c
		}
	}
	got, err := b.Facts().Recall(mine.Ctx, theirs.Scope, "fact", runtime.MaxMemoryFacts)
	if err == nil {
		assert.Empty(t, got, "another org's scope under this org's context finds nothing")
	} else {
		assert.True(t, memory.IsTenantMismatchError(err), "a refusal names the tenant mismatch: %v", err)
	}
	facts, _, _, err := b.Facts().List(mine.Ctx, theirs.Scope, "", 50)
	if err == nil {
		assert.Empty(t, facts)
	} else {
		assert.True(t, memory.IsTenantMismatchError(err), "%v", err)
	}
	id := listAll(t, b, theirs, theirs.Scope)[0].ID
	err = b.Facts().Delete(mine.Ctx, theirs.Scope, id)
	assert.True(t, memory.IsTenantMismatchError(err) || memory.IsNotFoundError(err), "%v", err)
	err = b.Facts().Record(mine.Ctx, theirs.Scope, memory.Turn{Facts: []string{"x"}, UserTexts: []string{"x"}})
	assert.True(t, memory.IsTenantMismatchError(err), "a record for another tenant is refused as such")
	assert.Len(t, listAll(t, b, theirs, theirs.Scope), 1)
}

func incomplete(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	s := b.Facts()
	noAgent := person(c.Scope)
	_, err := s.Recall(c.Ctx, noAgent, "", 5)
	incompleteScope(t, err, "agent", "recall names the agent")
	incompleteScope(t, s.Record(c.Ctx, noAgent, memory.Turn{Facts: []string{"x"}}), "agent", "record names the agent")
	noUser := c.Scope
	noUser.EndUserID = uuid.Nil
	_, err = s.Recall(c.Ctx, noUser, "", 5)
	incompleteScope(t, err, "end_user")
	incompleteScope(t, s.Record(c.Ctx, noUser, memory.Turn{Facts: []string{"x"}}), "end_user")
	_, _, _, err = s.List(c.Ctx, noUser, "", 10)
	incompleteScope(t, err, "end_user")
	incompleteScope(t, s.Delete(c.Ctx, noUser, "x"), "end_user")
	_, err = s.Clear(c.Ctx, noUser)
	incompleteScope(t, err, "end_user")
	noProject := c.Scope
	noProject.ProjectID = uuid.Nil
	_, err = s.Clear(c.Ctx, noProject)
	incompleteScope(t, err, "project")
	noStore := c.Scope
	noStore.StoreID = uuid.Nil
	_, err = s.Recall(c.Ctx, noStore, "", 5)
	incompleteScope(t, err, "store", "every call names the store")
	incompleteScope(t, s.Record(c.Ctx, noStore, memory.Turn{Facts: []string{"x"}, UserTexts: []string{"x"}}), "store")
	_, _, _, err = s.List(c.Ctx, person(noStore), "", 10)
	incompleteScope(t, err, "store")
	incompleteScope(t, s.Delete(c.Ctx, person(noStore), "x"), "store")
	_, err = s.Clear(c.Ctx, person(noStore))
	incompleteScope(t, err, "store")
}

func dedupe(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	record(t, b, c, "Lives in Bandung.", "lives in  BANDUNG.")
	record(t, b, c, " Lives in Bandung. ")
	assert.Equal(t, []string{"Lives in Bandung."}, texts(listAll(t, b, c, c.Scope)))
}

func prune(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	for i := range memory.MaxPerPerson / runtime.MaxExtractedFacts {
		batch := make([]string, runtime.MaxExtractedFacts)
		for j := range batch {
			batch[j] = fmt.Sprintf("fact %d", i*runtime.MaxExtractedFacts+j)
		}
		record(t, b, c, batch...)
	}
	record(t, b, c, "the newest fact")
	_, _, total, err := b.Facts().List(c.Ctx, c.Scope, "", 1)
	require.NoError(t, err)
	assert.Equal(t, memory.MaxPerPerson, total, "the oldest is dropped past the cap")
	got, err := b.Facts().Recall(c.Ctx, c.Scope, "fact 0", runtime.MaxMemoryFacts)
	require.NoError(t, err)
	assert.NotContains(t, texts(got), "fact 0", "the oldest went")
}

func restated(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	b.Seed(t, c, "Lives in Bandung.")
	for i := range memory.MaxPerPerson - 1 {
		b.Seed(t, c, fmt.Sprintf("fact %d", i))
	}
	run := b.Run(t, c)
	require.NoError(t, b.Facts().Record(c.Ctx, c.Scope, memory.Turn{RunID: run, Facts: []string{"lives in  BANDUNG."}}))
	record(t, b, c, "the newest fact")
	got := listAll(t, b, c, c.Scope)
	all, _, total, err := b.Facts().List(c.Ctx, c.Scope, "", memory.MaxPerPerson)
	require.NoError(t, err)
	assert.Equal(t, memory.MaxPerPerson, total)
	assert.Contains(t, texts(all), "Lives in Bandung.", "the person restated it, so the prune keeps it and drops the oldest other fact")
	assert.NotContains(t, texts(all), "fact 0")
	assert.Equal(t, "Lives in Bandung.", got[1].Text, "it is fresh: second only to the newest")
	assert.Equal(t, run, got[1].SourceRunID, "and names the run that restated it")
}

func oneWord(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	b.Seed(t, c, "Prefers WhatsApp after 6pm.", "Lives in Bandung.", "Has two cats.")
	got, err := b.Facts().Recall(c.Ctx, c.Scope, "hey, can you send the invoice to my whatsapp tomorrow morning?", 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"Prefers WhatsApp after 6pm."}, texts(got), "one shared word is enough")
}

func pad(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	b.Seed(t, c, "Lives in Bandung.")
	b.Seed(t, c, "Has two cats.")
	b.Seed(t, c, "Prefers tea.")
	got, err := b.Facts().Recall(c.Ctx, c.Scope, "bandung", 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"Lives in Bandung.", "Prefers tea."}, texts(got), "matches first, then the newest")
	got, err = b.Facts().Recall(c.Ctx, c.Scope, "", 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"Prefers tea.", "Has two cats."}, texts(got), "no query recalls the newest")
}

func list(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	for i := range 5 {
		b.Seed(t, c, fmt.Sprintf("fact %d", i))
	}
	page, next, total, err := b.Facts().List(c.Ctx, c.Scope, "", 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Equal(t, []string{"fact 4", "fact 3"}, texts(page))
	require.NotEmpty(t, next)
	page, next, _, err = b.Facts().List(c.Ctx, c.Scope, next, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"fact 2", "fact 1"}, texts(page))
	page, next, _, err = b.Facts().List(c.Ctx, c.Scope, next, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"fact 0"}, texts(page))
	assert.Empty(t, next, "the last page has no cursor")
}

func foreignCursor(t *testing.T, b Backend) {
	cells := b.Matrix(t)
	a, other := cells[0], cells[1]
	for i := range 3 {
		b.Seed(t, a, fmt.Sprintf("a %d", i))
		b.Seed(t, other, fmt.Sprintf("b %d", i))
	}
	_, next, _, err := b.Facts().List(a.Ctx, a.Scope, "", 1)
	require.NoError(t, err)
	page, _, _, err := b.Facts().List(other.Ctx, other.Scope, next, 10)
	if err == nil {
		for _, f := range page {
			assert.NotContains(t, f.Text, "a ", "a cursor never reveals another scope's facts")
		}
	}
}

func source(t *testing.T, b Backend) {
	cells := b.Matrix(t)
	c := cells[0]
	run := b.Run(t, c)
	require.NoError(t, b.Facts().Record(c.Ctx, c.Scope, memory.Turn{RunID: run, Facts: []string{"Lives in Bandung."}}))
	record(t, b, c, "Has two cats.")
	got := listAll(t, b, c, person(c.Scope))
	require.Len(t, got, 2)
	assert.Equal(t, c.Scope.AgentID, got[0].AgentID, "a person-wide list says which agent each fact belongs to")
	assert.Equal(t, uuid.Nil, got[0].SourceRunID)
	assert.Equal(t, run, got[1].SourceRunID)
	assert.False(t, got[1].CreatedAt.IsZero())
}

func mixedProject(t *testing.T, b Backend) {
	cells := seed(t, b)
	c := cells[0]
	var other Cell
	for _, o := range cells {
		if o.Scope.OrgID == c.Scope.OrgID && o.Scope.ProjectID != c.Scope.ProjectID {
			other = o
		}
	}
	mixed := c.Scope
	mixed.ProjectID = other.Scope.ProjectID
	got, err := b.Facts().Recall(c.Ctx, mixed, "fact", runtime.MaxMemoryFacts)
	require.NoError(t, err)
	assert.Empty(t, got, "a person and agent are never found under another project")
	facts, _, total, err := b.Facts().List(c.Ctx, person(mixed), "", 50)
	require.NoError(t, err)
	assert.Empty(t, facts)
	assert.Zero(t, total)
	id := listAll(t, b, c, c.Scope)[0].ID
	assert.True(t, memory.IsNotFoundError(b.Facts().Delete(c.Ctx, person(mixed), id)))
	n, err := b.Facts().Clear(c.Ctx, person(mixed))
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Len(t, listAll(t, b, c, c.Scope), 1)
}

func mixedStore(t *testing.T, b Backend) {
	cells := seed(t, b)
	c := cells[0]
	var other Cell
	for _, o := range cells {
		if o.Scope.ProjectID == c.Scope.ProjectID && o.Scope.StoreID != c.Scope.StoreID {
			other = o
		}
	}
	require.NotEqual(t, uuid.Nil, other.Scope.StoreID)
	mixed := c.Scope
	mixed.StoreID = other.Scope.StoreID
	b.Seed(t, Cell{Ctx: c.Ctx, Scope: mixed}, "kept in the other store")
	got, err := b.Facts().Recall(c.Ctx, c.Scope, "", runtime.MaxMemoryFacts)
	require.NoError(t, err)
	assert.Equal(t, []string{fact(c)}, texts(got), "the same person and agent recall nothing of another store")
	id := listAll(t, b, c, c.Scope)[0].ID
	assert.True(t, memory.IsNotFoundError(b.Facts().Delete(c.Ctx, person(mixed), id)), "a fact is not found through another store")
	n, err := b.Facts().Clear(c.Ctx, person(mixed))
	require.NoError(t, err)
	assert.Equal(t, 3, n, "clearing the other store takes only its own facts")
	assert.Len(t, listAll(t, b, c, c.Scope), 1)
}

func caps(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	facts := make([]string, 0, 12)
	facts = append(facts, strings.Repeat("é", 600))
	for i := range 11 {
		facts = append(facts, fmt.Sprintf("fact %d", i))
	}
	record(t, b, c, facts...)
	got := listAll(t, b, c, c.Scope)
	assert.Len(t, got, runtime.MaxExtractedFacts, "one turn keeps at most the cap, and one long fact never loses the others")
	for _, f := range got {
		assert.LessOrEqual(t, utf8.RuneCountInString(f.Text), runtime.MaxMemoryFactRunes)
	}
}

func oddQueries(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	b.Seed(t, c, "Lives in Bandung.")
	b.Seed(t, c, "Has two cats.")
	for _, q := range []string{"", "?!...", "it's", `a\b`, `'"quote"'`, string([]rune(strings.Repeat("kucing ŝ 猫 ", 250))[:2000])} {
		got, err := b.Facts().Recall(c.Ctx, c.Scope, q, 2)
		require.NoError(t, err, "%q", q)
		assert.Equal(t, []string{"Has two cats.", "Lives in Bandung."}, texts(got), "%q pads with the newest", q)
	}
}

func userTexts(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	require.NoError(t, b.Facts().Record(c.Ctx, c.Scope, memory.Turn{UserTexts: []string{"my number is [PHONE], call after 6"}}),
		"a driver that extracts accepts the person's masked texts")
}

func recordIncomplete(t *testing.T, b Backend) {
	c := b.Matrix(t)[0]
	noAgent := person(c.Scope)
	incompleteScope(t, b.Facts().Record(c.Ctx, noAgent, memory.Turn{UserTexts: []string{"x"}}), "agent")
}
