package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/project"
)

// ProjectStore is a project.Store test double. ByID/BySlug/List return Project/Projects (or Err); Save records the project and returns SaveFn/Err.
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
