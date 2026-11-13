package pgvector

import "altalune.id/oagent/internal/knowledge"

func Statement(r *Retriever, dims int) string { return r.statements[dims] }

var VersionSupported = versionSupported

// Keep ranks hits that carry no chunk id, which ties on score alone.
func Keep(hits []knowledge.Hit, minScore float64) []knowledge.Hit {
	rs := make([]ranked, len(hits))
	for i, h := range hits {
		rs[i] = ranked{hit: h}
	}
	return keep(rs, minScore, len(hits))
}
