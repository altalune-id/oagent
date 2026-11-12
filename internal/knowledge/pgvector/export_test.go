package pgvector

func Statement(r *Retriever, dims int) string { return r.statements[dims] }

var (
	Keep             = keep
	VersionSupported = versionSupported
)
