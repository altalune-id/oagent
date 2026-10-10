package session

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestReach(t *testing.T) {
	org, other := uuid.New(), uuid.New()
	p1, p2, res := uuid.New(), uuid.New(), uuid.New()

	person := Principal{Source: SourceLocal, UserID: uuid.New(), ActiveOrgID: org}
	assert.True(t, person.ReachesProject(org, p1))
	assert.False(t, person.ReachesProject(other, p1), "never across orgs")
	assert.False(t, person.ReachesProject(uuid.Nil, p1))

	basic := Principal{Source: SourceBasic, ActiveOrgID: org}
	assert.True(t, basic.ReachesProject(org, p2))

	key := Principal{Source: SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: org, ProjectIDs: []uuid.UUID{p1}}
	assert.True(t, key.ReachesProject(org, p1))
	assert.False(t, key.ReachesProject(org, p2))

	allKey := Principal{Source: SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: org, AllProjects: true}
	assert.True(t, allKey.ReachesProject(org, p2))

	narrow := Principal{Source: SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: org, ProjectIDs: []uuid.UUID{p1}, ResourceIDs: []uuid.UUID{res}}
	assert.True(t, narrow.ReachesResource(org, p1, res))
	assert.False(t, narrow.ReachesResource(org, p1, uuid.New()))
	assert.False(t, narrow.ReachesWholeProject(org, p1))
	assert.True(t, key.ReachesWholeProject(org, p1))
}

func TestReachesResource_NeedsProjectReach(t *testing.T) {
	org, p1, p2, res := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	key := Principal{Source: SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: org, ProjectIDs: []uuid.UUID{p1}, ResourceIDs: []uuid.UUID{res}}
	assert.False(t, key.ReachesResource(org, p2, res))
}
