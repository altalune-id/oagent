package legal

import (
	"net/http"

	"altalune.id/oagent/internal/legal/views"
	"altalune.id/oagent/internal/web/layout"
)

// WebOpts configures the legal pages.
type WebOpts struct {
	Kit        layout.Kit
	Service    *Service
	TermsURL   string
	PrivacyURL string
}

// Web serves /terms and /privacy.
type Web struct{ o WebOpts }

// NewWeb binds the legal pages to their options.
func NewWeb(o WebOpts) *Web {
	if o.Service == nil {
		o.Service = NewService()
	}
	return &Web{o: o}
}

// Register mounts the legal pages on mux.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+h.o.Kit.Path("/terms"), h.document(TermsSlug, h.o.TermsURL))
	mux.HandleFunc("GET "+h.o.Kit.Path("/privacy"), h.document(PrivacySlug, h.o.PrivacyURL))
}

func (h *Web) document(slug, override string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if override != "" {
			http.Redirect(w, r, override, http.StatusFound)
			return
		}
		doc, err := h.o.Service.Document(slug)
		if err != nil {
			h.o.Kit.Fail(w, r, "web: legal page", err)
			return
		}
		data := views.DocumentData{Title: doc.Title, HTML: doc.HTML, UpdatedAt: doc.UpdatedAt}
		h.o.Kit.Render(w, r, http.StatusOK, views.Document(h.o.Kit.Layout(doc.Title), data))
	}
}
