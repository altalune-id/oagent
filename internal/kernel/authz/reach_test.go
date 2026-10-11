package authz

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/session"
)

func TestReach(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	p1, p2, res := uuid.New(), uuid.New(), uuid.New()

	cases := []struct {
		name                                            string
		p                                               session.Principal
		org, wholeOrg, proj, wholeProj, resource, other bool
	}{
		{"no principal", session.Principal{}, false, false, false, false, false, false},
		{"system (stdio)", session.System(), true, true, true, true, true, true},
		{"operator", session.Principal{Source: session.SourceBasic, Operator: true, ActiveOrgID: orgB}, true, true, true, true, true, true},
		{"person in org A", session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA}, true, true, true, true, true, false},
		{"person elsewhere", session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgB}, false, false, false, false, false, true},
		{"org key, all projects", session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true}, true, true, true, true, true, false},
		{"org key, granted p1", session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{p1}}, true, false, true, true, true, false},
		{"project key p2", session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ActiveProjectID: p2, ProjectIDs: []uuid.UUID{p2}}, true, false, false, false, false, false},
		{"project key p1, one resource", session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{p1}, ResourceIDs: []uuid.UUID{res}}, true, false, true, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.p.Source != "" {
				ctx = session.PrincipalInto(ctx, tc.p)
			}
			assert.Equal(t, tc.org, ReachOrg(ctx, orgA), "org")
			assert.Equal(t, tc.wholeOrg, ReachWholeOrg(ctx, orgA), "whole org")
			assert.Equal(t, tc.proj, ReachProject(ctx, orgA, p1), "project")
			assert.Equal(t, tc.wholeProj, ReachWholeProject(ctx, orgA, p1), "whole project")
			assert.Equal(t, tc.resource, ReachResource(ctx, orgA, p1, res), "resource")
			assert.Equal(t, tc.other, ReachProject(ctx, orgB, p1), "another org")
		})
	}
}

func TestReach_NilOrgIsNeverReachedByAPrincipal(t *testing.T) {
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New()})
	assert.False(t, ReachOrg(ctx, uuid.Nil))
}
