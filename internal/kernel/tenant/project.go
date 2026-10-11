package tenant

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
)

// ProjectResolver finds the org a project belongs to.
type ProjectResolver interface {
	OrgOf(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)
}

// ProjectOutsideOrgError signals a project id that is not in the request's org, or does not exist.
type ProjectOutsideOrgError struct{}

func (*ProjectOutsideOrgError) Error() string { return "tenant: project is not in this organization" }

// IsProjectOutsideOrgError reports whether err wraps a *ProjectOutsideOrgError.
func IsProjectOutsideOrgError(err error) bool {
	_, ok := errors.AsType[*ProjectOutsideOrgError](err)
	return ok
}

// ToAppError maps ProjectOutsideOrgError to the validation envelope.
func (e *ProjectOutsideOrgError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeProjectNotInOrg, "Project is not in this organization").WithCause(e)
}

// RequireProjectInOrg fails unless projectID belongs to orgID; nil r skips it. SECURITY: a missing project gives the same error.
func RequireProjectInOrg(ctx context.Context, r ProjectResolver, orgID, projectID uuid.UUID) error {
	if r == nil {
		return nil
	}
	owner, err := r.OrgOf(ctx, projectID)
	if ae, ok := apperror.AsAppError(err); ok && apperror.KindOf(ae.Code()) == apperror.KindNotFound {
		return &ProjectOutsideOrgError{}
	}
	if err != nil {
		return err
	}
	if owner != orgID {
		return &ProjectOutsideOrgError{}
	}
	return nil
}
