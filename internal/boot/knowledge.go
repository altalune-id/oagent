package boot

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/kernel/storage/memory"
	storages3 "altalune.id/oagent/internal/kernel/storage/s3"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/knowledge/extract"
	"altalune.id/oagent/internal/knowledge/pgvector"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/run"
	s3conn "altalune.id/oagent/s3"
)

type knowledgeParts struct {
	driver   storage.Driver
	s3Health *s3conn.Health
	ingest   *knowledge.IngestWorker
	crawl    *knowledge.CrawlWorker
}

// NOTE: S3 is opened only when storage.driver is s3.
func buildKnowledge(ctx context.Context, k *kernel.Kernel, cfg config.Config, svcs *Services, log *slog.Logger) (knowledgeParts, error) {
	var parts knowledgeParts
	log = orDiscard(log)
	driver, health, err := buildStorageDriver(ctx, k, cfg, log)
	if err != nil {
		return parts, err
	}
	parts.driver, parts.s3Health = driver, health
	fileStore := storage.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix)
	files := storage.NewService(driver, fileStore, db.NewTransactor(k.Pool), storage.Config{
		Driver: string(cfg.Storage.Driver), Production: cfg.Env.IsProduction(), App: cfg.Storage.App,
		UploadURLTTL: cfg.Storage.Upload.URLTTL, PendingTTL: cfg.Storage.Upload.PendingTTL,
	}, k.Reporter.Unexpected)
	svcs.Files = files

	backends := make([]knowledge.Backend, 0, 1)
	for _, b := range cfg.Knowledge.Backends() {
		backends = append(backends, knowledge.Backend(b))
	}
	self, err := os.Executable()
	if err != nil {
		return parts, fmt.Errorf("boot: knowledge extract child: %w", err)
	}
	svc := knowledge.NewService(knowledge.NewStore(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix), db.NewTransactor(k.Pool),
		files, newPublicIDs(svcs), backends, k.Reporter.Unexpected).
		WithProjects(projectOrgs{projects: svcs.Projects}).
		WithModels(embeddingModels{providers: svcs.ModelProviders, store: svcs.ModelProviderStore}).
		WithBindings(knowledgeBindings{agents: svcs.AgentStore}).
		WithExtractor(knowledge.ChildExtractor{Child: extract.Child{Path: self, Args: []string{"extract"}}}).
		WithAllowPrivateHosts(knowledgePrivateHosts(cfg)).
		WithCrawlPrivateHosts(!cfg.Env.IsProduction())
	if cfg.Knowledge.AllowPrivateHosts && cfg.Env.IsProduction() {
		log.Warn("boot: knowledge.allowPrivateHosts is on in production; any project's embeddings provider URL can reach private hosts",
			"use", "single-tenant installs only")
	}
	if cfg.Knowledge.Pgvector.Enabled {
		if err := pgvector.CheckVersion(ctx, k.Pool.W); err != nil {
			return parts, fmt.Errorf("boot: %w", err)
		}
	}
	svcs.KnowledgeRetriever = pgvector.New(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix, svc)
	svc.WithRetrievers(map[knowledge.Backend]knowledge.Retriever{knowledge.BackendPgvector: svcs.KnowledgeRetriever})
	tenants := outboxTenants{orgs: svcs.Orgs}
	parts.ingest = knowledge.NewIngestWorker(svc, tenants, log)
	parts.crawl = knowledge.NewCrawlWorker(svc, tenants, log)
	svc.WithKick(parts.ingest.Kick).WithCrawlKick(parts.crawl.Kick)
	svcs.Knowledge = svc

	svcs.Agents.WithKnowledge(grantKnowledge{knowledge: svc})
	svcs.ModelProviders.WithUsage(providerUsage{agents: svcs.AgentStore, knowledge: svc})
	return parts, nil
}

// SECURITY: private-network embeddings hosts only outside production or with knowledge.allowPrivateHosts; never the crawler.
func knowledgePrivateHosts(cfg config.Config) bool {
	return cfg.Knowledge.AllowPrivateHosts || !cfg.Env.IsProduction()
}

type runKnowledge struct {
	agents    agent.Store
	knowledge *knowledge.Service
}

// BoundSources implements run.KnowledgeBindings. SECURITY: drops gone sources and any outside the version's project.
func (r runKnowledge) BoundSources(ctx context.Context, versionID uuid.UUID) ([]run.KnowledgeSource, error) {
	v, err := r.agents.VersionByID(ctx, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]run.KnowledgeSource, 0, len(v.Config.KnowledgeSourceIDs))
	for _, id := range v.Config.KnowledgeSourceIDs {
		src, err := r.knowledge.GetSource(ctx, id)
		if knowledge.IsSourceNotFoundError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if src.OrgID != v.OrgID || src.ProjectID != v.ProjectID {
			continue
		}
		out = append(out, run.KnowledgeSource{ID: src.ID, PublicID: src.PublicID})
	}
	return out, nil
}

func buildStorageDriver(
	ctx context.Context, k *kernel.Kernel, cfg config.Config, log *slog.Logger,
) (storage.Driver, *s3conn.Health, error) {
	switch cfg.Storage.Driver {
	case "":
		return nil, nil, nil
	case config.StorageDriverMemory:
		return memory.New(), nil, nil
	case config.StorageDriverS3:
		c := cfg.S3
		client, err := s3conn.Open(ctx, s3conn.Config{
			Endpoint: c.Endpoint, BrowserEndpoint: c.BrowserEndpoint, Region: c.Region, Bucket: c.Bucket,
			AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, UsePathStyle: c.UsePathStyle, Timeout: c.Timeout,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("boot: %w", err)
		}
		k.AddCloser(client)
		return storages3.New(client), s3conn.NewHealth(client, c.Timeout, log), nil
	default:
		return nil, nil, fmt.Errorf("boot: unknown storage.driver %q", cfg.Storage.Driver)
	}
}

type embeddingModels struct {
	providers *modelprovider.Service
	store     modelprovider.Store
}

// LockProvider implements knowledge.Models: the provider stays as it is until the unit of work on ctx ends.
func (m embeddingModels) LockProvider(ctx context.Context, providerID uuid.UUID) error {
	return m.store.LockShared(ctx, []uuid.UUID{providerID})
}

// EmbeddingModel implements knowledge.Models over the project's model providers. SECURITY: the result carries the opened API key.
func (m embeddingModels) EmbeddingModel(ctx context.Context, providerID uuid.UUID, model string) (knowledge.EmbeddingModel, error) {
	r, err := m.providers.ResolveModel(ctx, providerID, model)
	if err != nil {
		return knowledge.EmbeddingModel{}, err
	}
	out := knowledge.EmbeddingModel{
		OpenAICompatible: r.Kind == modelprovider.KindOpenAICompatible, BaseURL: r.BaseURL, APIKey: r.APIKey, Embedding: r.Model.Embedding,
	}
	if r.Model.Prices != nil {
		out.InputPriceMicros = new(r.Model.Prices.InputMicros)
	}
	return out, nil
}
