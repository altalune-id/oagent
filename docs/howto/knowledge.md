# Turn on knowledge sources (operator)

Use this to let agents answer from a project's documents and websites. Agents search their bound sources before each turn; the runtime adds the best passages to the model input, and nothing about them is stored in the thread.

## What a deployment needs

| Need                                | Setting                                                                                                                                                                                                                                                                          | Without it                                    |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| a search backend                    | `knowledge.pgvector.enabled: true`                                                                                                                                                                                                                                               | no source can be created                      |
| the `vector` extension              | pgvector 0.8 or newer in the database (step 1)                                                                                                                                                                                                                                   | migration 31 fails                            |
| object storage                      | `storage.driver: s3` and `s3.*` (step 2)                                                                                                                                                                                                                                         | uploads answer KNW009; URL sources still work |
| an embedding model                  | set per project in the console (Knowledge) or `SetEmbeddingModel`                                                                                                                                                                                                                | documents wait as pending (KNW011)            |
| a private-network embeddings server | `knowledge.allowPrivateHosts: true` in production, **single-tenant installs only**: any project's provider base URL can then reach private hosts, and boot logs a WARN. Outside production it is allowed without the flag. The crawler never reaches private hosts in production | KNW013 `unreachable`                          |

## Steps

1. **Create the extension.** Every install needs it, with or without knowledge: migration 31 runs `CREATE EXTENSION`. `scripts/db/provision.sh` does it for the databases it provisions (and the local compose image ships pgvector). For a database it doesn't provision (Neon), create it once, as an admin or the database owner, before oagent migrates:

   ```sql
   CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
   SELECT extversion FROM pg_extension WHERE extname = 'vector'; -- 0.8.0 or newer
   ```

   The migration repeats the `CREATE EXTENSION`, which is a no-op once it exists. Rolling migration 31 back keeps the extension.

   | Error at migrate                                 | Fix                                                                                                      |
   | ------------------------------------------------ | -------------------------------------------------------------------------------------------------------- |
   | `permission denied to create extension "vector"` | the migrator may not create extensions; run the `CREATE EXTENSION` above as an admin, then migrate again |
   | `extension "vector" is not available`            | the server has no pgvector; install it, or use a Postgres that ships it                                  |

2. **Storage.** Point oagent at an S3-compatible bucket (R2 in production, MinIO or Garage locally):

   | Key                                    | Example                                                      |
   | -------------------------------------- | ------------------------------------------------------------ |
   | `storage.driver`                       | `s3`                                                         |
   | `storage.app`                          | `oagent` (the key folder; apps can share a bucket)           |
   | `s3.endpoint`                          | `https://<account>.r2.cloudflarestorage.com`                 |
   | `s3.region`                            | `auto` for R2                                                |
   | `s3.bucket`                            | the bucket name                                              |
   | `s3.accessKeyID`, `s3.secretAccessKey` | an R2 API token with Object Read & Write on that bucket only |

   Then allow the console origin in the bucket's CORS rules, method `PUT`, header `Content-Type`. Browsers upload straight to the bucket with a presigned URL valid for `storage.upload.urlTTL` (15 minutes); an upload left unfinished for `storage.upload.pendingTTL` (1 hour) is swept.

3. **Enable the backend:** `knowledge.pgvector.enabled: true`, then restart. The `knowledge-ingest` and `knowledge-crawl` workers start.
4. **Per project,** mark a model of an OpenAI-compatible provider as an embedding model and give it an input price (0 is fine for a self-hosted model), then pick it on the Knowledge page. Its vector size must be 768, 1024, 1536 or 3072. Changing the model re-embeds every document.

## Formats and caps

| What                 | Limit                                                                                        |
| -------------------- | -------------------------------------------------------------------------------------------- |
| uploads              | PDF with a text layer, DOCX, Markdown, plain text (UTF-8)                                    |
| one file             | 10 MiB                                                                                       |
| documents per source | 100 (failed ones don't count)                                                                |
| text per document    | 2 MiB once extracted; 2,000 passages                                                         |
| passages per project | 20,000                                                                                       |
| a website source     | 50 pages on the seed's host, depth 2, https only, robots.txt respected, 1 request per second |
| a crawled page       | 2 MiB of extracted text as JSON; larger pages are skipped                                    |
| sources per project  | 50                                                                                           |
| refreshing a source  | once every 10 minutes                                                                        |

Scanned PDFs have no text layer and fail with KNW005. A seed that redirects to another host (for example to `www.`) fails with KNW014; paste the final address.

## Retrieval tuning per agent

Each agent version may tune its knowledge search (`AgentConfig.knowledge_tuning`). Changes publish like any other config change. A value left at 0 or unset follows the current recommended value at run time, so a new recommended value reaches live agents without a republish.

| Setting             | Field        | Recommended | Range    |
| ------------------- | ------------ | ----------- | -------- |
| Passages per answer | `top_k`      | 5           | 1-10     |
| Minimum relevance   | `min_score`  | 0.20        | 0-1      |
| Knowledge budget    | `max_tokens` | 4000        | 500-8000 |

Presets: precise 3 / 0.35, balanced 5 / 0.20 (the recommended values), broad 8 / 0.10. `ListKnowledgeSources` returns all of these in `retrieval_defaults`. Test search always uses the recommended values.

## Reference code

- [internal/knowledge](../../internal/knowledge): service, ingest and crawl workers, `controlplane.go`
- [internal/kernel/storage](../../internal/kernel/storage): uploads, verification, sweep
- [internal/boot/knowledge.go](../../internal/boot/knowledge.go): the wiring
