package health

import "net/http"

// Web serves GET /healthz and /readyz. SECURITY: probes are root-anchored and carry no state, so they skip auth.
type Web struct{ svc *Service }

// NewWeb binds the probes to svc; nil svc is always ready.
func NewWeb(svc *Service) *Web {
	if svc == nil {
		svc = NewService()
	}
	return &Web{svc: svc}
}

// Register mounts the probes on mux, outside the base path.
func (h *Web) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
}

func (h *Web) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Web) readyz(w http.ResponseWriter, _ *http.Request) {
	if !h.svc.Ready() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
