# Project overview

You’ve got four historic document datasets (decision notices, land charges cards, enforcement notices, agreements) in `source_docs/`, many lacking **UPRN/Easting/Northing**. You also have a **current LLPG** already loaded into **PostGIS** via Docker Compose (service `db`, database `ehdc_gis`, user/pass in compose) — this is your **authoritative address universe**and the place to source canonical coordinates and metadata. 

The goal is to:

1. ingest and normalise those four sources into a unified “documents” table,
    
2. match each record to the **best modern UPRN** from LLPG, and
    
3. back-fill **Easting/Northing** from LLPG with full auditability.
    

Two artefacts drive this:

- **Data specification & schema** (staging, core dimension, matching/audit tables).
    
- **Advanced matching engine** (deterministic + fuzzy + embeddings + phonetics + spatial + explainable scoring).
    

These are deliberately decoupled so the matching can iterate without changing the storage model; every decision is recorded, reproducible, and reviewable.

---

# How the specification and algorithm tie together

**dim_address (LLPG)** is the “truth” — UPRN, `locaddress`, `easting`, `northing`, `usrn`, class, geometries.  
**src_document** holds the normalised historic addresses (`raw_address`, `addr_can`, optional legacy UPRN and raw coordinates).  
**match_result / match_accepted / match_run** record how each suggestion was produced, with features, scores, methods and thresholds.

The **algorithm** consumes `src_document` rows and the indexed `dim_address` universe to produce ranked candidates:

1. **Deterministic passes**
    
    - Validate any legacy UPRN directly against LLPG.
        
    - Canonical exact address match.
        
2. **Fuzzy & semantic passes**
    
    - PostgreSQL **pg_trgm** similarity on `addr_can`.
        
    - **Phonetic** filters (Double Metaphone) for street/locality tokens.
        
    - **Locality & house-number** overlap filters.
        
    - **Embeddings** via a local model + **Qdrant** ANN to catch hard string differences.
        
    - **Spatial** narrowing if the source carries E/N.
        
3. **Scoring & decision**
    
    - Blend features (trigram, embedding cosine, token overlaps, phonetics, spatial boost, USRN/status hints) into a **meta-score** with safe auto-accept thresholds, else queue for review.
        
    - Accepted links populate `match_accepted`, and downstream views/exports back-fill **UPRN/E/N** onto the source datasets.
        

Because the **spec** captures each attempt, tie, decision and override, you can re-run improved match logic any time and still preserve prior manual acceptances.

---

# System architecture at a glance

- **Docker**: PostGIS is already up (port **15432** → container 5432, DB **ehdc_gis**). 
    
- **PostgreSQL extensions**: PostGIS, `pg_trgm`, `unaccent`.
    
- **Vector search**: Qdrant (Docker) — for address embeddings.
    
- **Embeddings**: local (e.g., Ollama or TEI) — free models like `all-MiniLM-L6-v2` / `bge-small-en`.
    
- **Normalisation**: libpostal (sidecar) + your UK rules.
    
- **Go services**: ETL loader, match engine, reporting; all instrumented with your `debug` pattern.
    

---

# Plan for an AI coding agent

Below is a task-oriented plan the agent can execute in small, verifiable steps. It assumes the repo root contains `docker-compose.yml` (PostGIS), and the CSVs are under `./source_docs/`.

## Phase 0 — Bootstrap & conventions

**Objectives**

- Establish project structure.
    
- Provide connection details to DB from compose.
    

**Actions**

- Create folders:
    
    `/cmd/matcher /internal/db        (migrations, queries) /internal/normalize /internal/generator (pg_trgm, vector, phonetic, spatial) /internal/rank      (features & scoring) /internal/report /configs /scripts`
    
- Add `.env` with:
    
    `PGHOST=localhost PGPORT=15432 PGDATABASE=ehdc_gis PGUSER=user PGPASSWORD=password`
    
    (values align with your compose) 
    
- Wire `internal/db/connect.go` using `pgx` and your `debug` hooks.
    

**Acceptance**

- `go run ./cmd/matcher --ping-db` connects successfully.
    

---

## Phase 1 — Schema migrations (data spec)

**Objectives**

- Create tables for `src_document`, `match_run`, `match_result`, `match_accepted`, `match_override`, and helper indexes; assume `dim_address` (LLPG) already exists.
    

**Actions**

- Add SQL migrations for:
    
    - `src_document` (unified ingestion: source type, raw/canonical address, extracted postcode, legacy UPRN, raw E/N, file path, job number).
        
    - `match_*` tables and indexes (GIN on `addr_can` for trgm; GIST on `geom27700` exists in dim).
        
    - Ensure `pg_trgm` and `unaccent` extensions.
        
- Add `make migrate-up` task.
    

**Acceptance**

- Running migrations creates tables and indexes with no errors.
    

---

## Phase 2 — Ingest & normalise the four source CSVs

**Objectives**

- Load all CSVs from `./source_docs/` into `src_document`.
    
- Produce consistent `addr_can` and `postcode_text`.
    

**Actions**

- Implement `normalize.CanonicalAddress()` in Go (with your `debug` logging).
    
- A small loader CLI:
    
    `go run ./cmd/matcher ingest --dir ./source_docs --source decision go run ./cmd/matcher ingest --dir ./source_docs --source land_charge go run ./cmd/matcher ingest --dir ./source_docs --source enforcement go run ./cmd/matcher ingest --dir ./source_docs --source agreement`
    
    The loader maps columns per source, extracts `postcode_text`, and stores `addr_can`.
    

**Acceptance**

- Row counts in `src_document` match input totals; spot check 10 rows per source for correct canonicalisation.
    

---

## Phase 3 — Deterministic matching

**Objectives**

- Fast wins: validate legacy UPRN; canonical exact matches.
    

**Actions**

- `match run --label v1-deterministic`:
    
    - For rows with `uprn_raw` present and in `dim_address.uprn`: write `match_result` (method `valid_uprn`, score `1.0`) and `match_accepted`.
        
    - For rows without acceptance: join `addr_can` == `dim_address.addr_can` → accept if unique; else write N candidates with `needs_review`.
        

**Acceptance**

- Coverage increases to baseline deterministic levels; `report summary --label v1-deterministic` shows counts by method.
    

---

## Phase 4 — Fuzzy (pg_trgm), phonetic & rule filters

**Objectives**

- Widen candidate net while keeping precision high.
    

**Actions**

- Add GIN trigram index on `dim_address.addr_can`.
    
- Implement generator: `SELECT ... WHERE addr_can %% $1 ORDER BY similarity($1, addr_can) DESC LIMIT 200`.
    
- Add phonetic filter (Double Metaphone) and locality/house-number filters before keeping candidates.
    
- Write all candidates to `match_result` with method `trgm` and their `trgm` score feature.
    

**Acceptance**

- `report candidates --src-id <id>` shows sensible top-N with string sims and filtered junk removed.
    

---

## Phase 5 — Embeddings + Qdrant (semantic candidates)

**Objectives**

- Catch hard variants and legacy spellings.
    

**Actions**

- Stand up Qdrant in Docker; index embeddings for all `dim_address.addr_can`.
    
- Run a local embedding model (Ollama or TEI).
    
- For each unmatched source, embed `addr_can` and query top-K in Qdrant; merge unique UPRNs into candidate set with `embed_cos` feature.
    

**Acceptance**

- For known tricky examples, semantic candidates appear in top-10.
    

---

## Phase 6 — Spatial hinting

**Objectives**

- Use any available raw E/N from sources to down-weight far candidates.
    

**Actions**

- If a source row has E/N, compute distance to candidate’s LLPG point (ST_Distance on SRID:27700).
    
- Add `spatial_boost = exp(-dist_m / 300)` into features.
    

**Acceptance**

- Nearby candidates get visible score lift; distant ones drop.
    

---

## Phase 7 — Meta-scoring & decisions

**Objectives**

- Blend features into a stable, explainable score; decide auto-accept/review/reject.
    

**Actions**

- Implement `rank.Score()` with the initial weights/thresholds; enforce:
    
    - Auto-accept if `score ≥ 0.92` and margin ≥ 0.03 (or 0.88 with house-number & locality overlap).
        
    - Queue for review if 0.80–auto.
        
    - Reject if < 0.80.
        
- Write final decisions to `match_accepted`.
    

**Acceptance**

- `report summary --label v2-fuzzy+semantic` shows uplift and method breakdown; spot checks confirm precision of auto-accepts.
    

---

## Phase 8 — Reporting, exports, and back-fill

**Objectives**

- Produce datasets with appended UPRN/E/N; surface QA metrics.
    

**Actions**

- Create a view `vw_src_with_match` joining `src_document` → `match_accepted` → `dim_address` to expose authoritative UPRN/E/N and the `matched_by` method and `score`.
    
- Export per-source CSVs to `out/` for downstream use.
    
- Summary KPIs: coverage %, auto-accept precision (sample audit), unresolved count, top reasons for ambiguity.
    

**Acceptance**

- Exports exist and open cleanly; metrics presented in a short HTML/Markdown report.
    

---

## Phase 9 — Human-in-the-loop review & learning (optional but recommended)

**Objectives**

- Close the last mile and continuously improve.
    

**Actions**

- Generate a reviewer CSV (or minimal web UI) listing ambiguous rows with top-3 candidates and buttons to accept/reject.
    
- Accepted decisions write to `match_accepted` (method `manual`).
    
- Periodically fit a simple logistic model on the features to refine weights; export to ONNX and load in Go for production scoring.
    

**Acceptance**

- Ambiguous rows can be cleared quickly; new weights slightly improve auto-accept recall without hurting precision.
    

---

# Repository layout (proposed)

	/cmd/matcher            # CLI entrypoints: ingest, match run, report, export
	/internal/db            # connect, migrations, queries
	/internal/normalize     # CanonicalAddress, tokenisers, abbrev rules
	/internal/generator     # pg_trgm, vector ANN, phonetic, spatial filters
	/internal/rank          # features + scoring + thresholds
	/internal/report        # summaries, exports
	/configs                # model names, thresholds, rules
	/scripts                # make targets, helper scripts
	/source_docs/           # your CSV inputs (read-only to app)
	/out/                   # exports and reports

`

---

# Configuration & connections

- **DB (from compose)**  
    Host `localhost`, port **15432**, db `ehdc_gis`, user `user`, password `password`.   
    Example Go DSN (pgx):  
    `postgres://user:password@localhost:15432/ehdc_gis?sslmode=disable`
    
- **Embeddings**  
    `EMBED_PROVIDER=ollama` (or `tei`)  
    `EMBED_MODEL=all-minilm-l6-v2` (or `bge-small-en`)
    
- **Qdrant**  
    `QDRANT_URL=http://localhost:6333`  
    Collection: `llpg_addr_can`
    
- **Rules & thresholds**  
    Keep in `/configs/` (YAML): abbreviations, locality list, decision thresholds.
    

---

# Data flow summary

1. **Load** `source_docs` → `src_document` with canonicalisation.
    
2. **Deterministic** matches (legacy UPRN validation, exact addr_can).
    
3. **Fuzzy** candidates (pg_trgm) → filtered by phonetics/locality/house number.
    
4. **Semantic** candidates (embeddings via Qdrant).
    
5. **Spatial** boost (if E/N present on source).
    
6. **Blend** features → ranked candidates → decision.
    
7. **Write** `match_accepted`, export joined UPRN/E/N; log everything in `match_result`.
    

---

# Runbook (typical commands)

	# 0) DB up (you already have this)
	docker compose up -d  # PostGIS with LLPG loaded
	
	# 1) Migrations
	make migrate-up
	
	# 2) Ingest sources
	go run ./cmd/matcher ingest --dir ./source_docs --source decision
	go run ./cmd/matcher ingest --dir ./source_docs --source land_charge
	go run ./cmd/matcher ingest --dir ./source_docs --source enforcement
	go run ./cmd/matcher ingest --dir ./source_docs --source agreement
	
	# 3) Deterministic pass
	go run ./cmd/matcher match run --label v1-deterministic
	
	# 4) Bring up Qdrant + embeddings (docker)
	docker compose -f docker-compose.vector.yml up -d
	go run ./cmd/matcher index-llpg-vectors
	go run ./cmd/matcher match run --label v2-fuzzy+semantic
	
	# 5) Reports & exports
	go run ./cmd/matcher report summary --label v2-fuzzy+semantic
	go run ./cmd/matcher export --dir ./out


---

# Acceptance metrics

- Auto-accept **precision ≥ 98%** in spot checks.
    
- Coverage uplift reports by method: `valid_uprn`, `addr_exact`, `trgm`, `vector`, `hybrid`, `manual`.
    
- 100% of accepted links carry LLPG-sourced **Easting/Northing**.
    
- Every decision is explainable (features + score) and reproducible (`match_run` versioned).