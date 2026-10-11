package authz

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/session"
)

// ReachOrg reports whether the caller in ctx may read inside orgID. SECURITY: no Principal reaches nothing; stdio runs as System().
func ReachOrg(ctx context.Context, orgID uuid.UUID) bool {
	p := session.PrincipalFrom(ctx)
	return trusted(p) || (orgID != uuid.Nil && orgID == p.ActiveOrgID)
}

// ReachWholeOrg reports whether the caller in ctx may act across all of orgID, as an org-level write does.
func ReachWholeOrg(ctx context.Context, orgID uuid.UUID) bool {
	p := session.PrincipalFrom(ctx)
	if trusted(p) {
		return true
	}
	if !ReachOrg(ctx, orgID) {
		return false
	}
	return p.Source != session.SourceAPIKey || (p.AllProjects && len(p.ResourceIDs) == 0)
}

// ReachProject reports whether the caller in ctx may act in projectID of orgID.
func ReachProject(ctx context.Context, orgID, projectID uuid.UUID) bool {
	p := session.PrincipalFrom(ctx)
	return trusted(p) || p.ReachesProject(orgID, projectID)
}

// ReachWholeProject reports whether the caller in ctx may act across every resource of projectID, as a create does.
func ReachWholeProject(ctx context.Context, orgID, projectID uuid.UUID) bool {
	p := session.PrincipalFrom(ctx)
	return trusted(p) || p.ReachesWholeProject(orgID, projectID)
}

// ReachResource reports whether the caller in ctx may act on resourceID inside projectID of orgID.
func ReachResource(ctx context.Context, orgID, projectID, resourceID uuid.UUID) bool {
	p := session.PrincipalFrom(ctx)
	return trusted(p) || p.ReachesResource(orgID, projectID, resourceID)
}

// SECURITY: a person reaches only their active org (P1); operators reach every org and keys are never operators.
func trusted(p session.Principal) bool {
	return p.Operator
}
