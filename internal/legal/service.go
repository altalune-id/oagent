package legal

// Service serves the legal documents.
type Service struct{}

// NewService builds a Service.
func NewService() *Service { return &Service{} }

// Document returns the document for slug.
func (s *Service) Document(slug string) (*Document, error) {
	return BySlug(slug)
}
