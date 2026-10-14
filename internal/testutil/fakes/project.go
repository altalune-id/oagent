package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/project"
)

// ProjectStore is a project.Store test double that returns its configured values or Err.
type ProjectStore struct {
	Err      error
	Project  *project.Project
	Projects []*project.Project
	SaveFn   func(*project.Project) error
	Saved    *project.Project
}

var _ project.Store = (*ProjectStore)(nil)

// Save implements project.Store.
func (f *ProjectStore) Save(_ context.Context, p *project.Project) error {
	f.Saved = p
	if f.SaveFn != nil {
		return f.SaveFn(p)
	}
	return f.Err
}

// ByID implements project.Store.
func (f *ProjectStore) ByID(context.Context, uuid.UUID) (*project.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Project, nil
}

// ByPublicID implements project.Store; it finds Project or one of Projects by public id.
func (f *ProjectStore) ByPublicID(_ context.Context, publicID string) (*project.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	for _, p := range append([]*project.Project{f.Project}, f.Projects...) {
		if p != nil && p.PublicID == publicID {
			return p, nil
		}
	}
	return nil, &project.NotFoundError{ID: publicID}
}

// BySlug implements project.Store.
func (f *ProjectStore) BySlug(context.Context, uuid.UUID, string) (*project.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Project, nil
}

// List implements project.Store.
func (f *ProjectStore) List(context.Context, uuid.UUID) ([]*project.Project, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Projects, nil
}
