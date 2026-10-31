package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatalog(t *testing.T) {
	assert.Contains(t, Names(), "org:read")
	assert.Contains(t, Names(), "agent:write")
	assert.True(t, Valid("agent:read"))
	assert.False(t, Valid("nope"))

	level, ok := LevelOf("org:write")
	assert.True(t, ok)
	assert.Equal(t, LevelOrg, level)

	level, ok = LevelOf("agent:read")
	assert.True(t, ok)
	assert.Equal(t, LevelProject, level)

	_, ok = LevelOf("nope")
	assert.False(t, ok)

	assert.Len(t, All(), len(Names()))
}

func TestEverySpecHasLevel(t *testing.T) {
	for _, s := range All() {
		assert.Contains(t, []Level{LevelOrg, LevelProject}, s.Level, s.Name)
	}
}

func TestCatalog_Holders(t *testing.T) {
	for _, s := range All() {
		switch s.Name {
		case MemberWrite:
			assert.Equal(t, HolderPerson, s.Holder, s.Name)
		case ApikeyWrite:
			assert.Equal(t, HolderConsole, s.Holder, s.Name)
		default:
			assert.Equal(t, HolderAny, s.Holder, s.Name)
		}
	}
}

func TestHolds(t *testing.T) {
	for name, tc := range map[string]struct {
		scope string
		by    Holder
		want  bool
	}{
		"key, any scope":          {MemberRead, HolderAny, true},
		"key, person scope":       {MemberWrite, HolderAny, false},
		"key, console scope":      {ApikeyWrite, HolderAny, false},
		"token, any scope":        {AgentWrite, HolderPerson, true},
		"token, person scope":     {MemberWrite, HolderPerson, true},
		"token, console scope":    {ApikeyWrite, HolderPerson, false},
		"console, any scope":      {AgentRead, HolderConsole, true},
		"console, person scope":   {MemberWrite, HolderConsole, true},
		"console, console scope":  {ApikeyWrite, HolderConsole, true},
		"outside the catalog":     {"nope:read", HolderConsole, false},
		"an unknown holder holds": {AgentRead, Holder("root"), false},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, Holds(tc.scope, tc.by)) })
	}
}

func TestParse_RefusesAnUnknownHolder(t *testing.T) {
	_, err := parse([]byte(`[{"name":"a:read","level":"org","holder":"any"}]`))
	assert.ErrorContains(t, err, "a:read")

	specs, err := parse([]byte(`[{"name":"a:read","level":"org","holder":"console"}]`))
	assert.NoError(t, err)
	assert.Equal(t, HolderConsole, specs[0].Holder)
}

func TestGrants(t *testing.T) {
	for name, tc := range map[string]struct {
		granted  []string
		required string
		want     bool
	}{
		"exact":             {[]string{AgentRead}, AgentRead, true},
		"other":             {[]string{AgentRead}, AgentWrite, false},
		"star":              {[]string{"*"}, MemberWrite, true},
		"resource wildcard": {[]string{"agent:*"}, AgentWrite, true},
		"wildcard no leak":  {[]string{"agent:*"}, ProjectRead, false},
		"prefix trick":      {[]string{"agent:*"}, "agents:read", false},
		"empty":             {nil, AgentRead, false},
		"empty required":    {[]string{"*"}, "", false},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tc.want, Grants(tc.granted, tc.required)) })
	}
}

func TestCovers(t *testing.T) {
	admin := []string{OrgRead, "project:*", "agent:*", "webhook:*", "member:*", "apikey:*"}
	plain := []string{OrgRead, ProjectRead, "agent:*", "webhook:*", MemberRead, ApikeyRead}
	assert.True(t, Covers([]string{"*"}, admin), "* covers everything")
	assert.True(t, Covers(admin, admin), "a set covers itself")
	assert.True(t, Covers(admin, plain))
	assert.False(t, Covers(admin, []string{"*"}), "admin lacks org:write")
	assert.False(t, Covers(plain, admin))
	assert.True(t, Covers(plain, nil), "anything covers nothing")
	assert.False(t, Covers(nil, []string{MemberRead}))
	assert.True(t, Covers([]string{AgentRead, AgentWrite}, []string{"agent:*"}), "a wildcard is the catalog permissions it names")
	assert.False(t, Covers([]string{AgentRead}, []string{"agent:*"}))
}
