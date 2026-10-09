package entity_test

import (
	"strings"
	"sync"
	"testing"

	jetpg "github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/schema/entity"
)

func TestNullHelpersCarryTheColumnType(t *testing.T) {
	cases := []struct {
		name string
		expr jetpg.Expression
		want string
	}{
		{"NullText", entity.NullText(), `SELECT NULL::text AS "v";`},
		{"NullDate", entity.NullDate(), `SELECT NULL::date AS "v";`},
		{"NullTimestampz", entity.NullTimestampz(), `SELECT NULL::timestamp with time zone AS "v";`},
		{"NullUUID", entity.NullUUID(), `SELECT NULL::uuid AS "v";`},
		{"NullJSONB", entity.NullJSONB(), `SELECT NULL::jsonb AS "v";`},
		{"NullBytea", entity.NullBytea(), `SELECT NULL::bytea AS "v";`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			query, _ := jetpg.SELECT(tt.expr.AS("v")).Sql()
			assert.Equal(t, tt.want, strings.TrimSpace(query))
		})
	}
}

func TestNullHelpersAreConcurrencySafe(t *testing.T) {
	builders := []func() jetpg.Expression{
		func() jetpg.Expression { return entity.NullText() },
		func() jetpg.Expression { return entity.NullDate() },
		func() jetpg.Expression { return entity.NullTimestampz() },
		func() jetpg.Expression { return entity.NullUUID() },
		func() jetpg.Expression { return entity.NullJSONB() },
		func() jetpg.Expression { return entity.NullBytea() },
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			build := builders[i%len(builders)]
			for range 200 {
				query, _ := jetpg.SELECT(build().AS("v")).Sql()
				assert.Contains(t, query, "NULL")
			}
		})
	}
	wg.Wait()
}

func TestNewOrgs(t *testing.T) {
	orgs := entity.NewOrgs("auth", "t_")
	query, _ := jetpg.SELECT(orgs.PublicID).FROM(orgs).Sql()
	assert.Contains(t, query, "t_orgs")
	assert.Contains(t, query, "public_id")

	// Empty schema falls back to public.
	def := entity.NewOrgs("", "")
	query, _ = jetpg.SELECT(def.ID).FROM(def).Sql()
	assert.Contains(t, query, "orgs")
}

func TestNewProjects(t *testing.T) {
	projects := entity.NewProjects("auth", "t_")
	query, _ := jetpg.SELECT(projects.Slug).FROM(projects).Sql()
	assert.Contains(t, query, "t_projects")
	assert.Contains(t, query, "slug")

	// Empty schema falls back to public.
	def := entity.NewProjects("", "")
	query, _ = jetpg.SELECT(def.ID).FROM(def).Sql()
	assert.Contains(t, query, "projects")
}

func TestNewAgents(t *testing.T) {
	agents := entity.NewAgents("auth", "t_")
	query, _ := jetpg.SELECT(agents.Description).FROM(agents).Sql()
	assert.Contains(t, query, "t_agents")
	assert.Contains(t, query, "description")

	// Empty schema falls back to public.
	def := entity.NewAgents("", "")
	query, _ = jetpg.SELECT(def.ID).FROM(def).Sql()
	assert.Contains(t, query, "agents")
}
