Here’s a complete, practical plan to get every record populated with **UPRN, Easting, and Northing**, based on the files you shared. I’ve actually inspected the data you uploaded and sized the problem so the specification below is grounded in your real columns and data quality.

---

# What I looked at (quick facts)

I analysed five files:

- `decision_notices.csv` — 76,167 rows, columns include `Job Number`, `Filepath`, `Planning Application Number`, `Adress` (note the misspelling), `Decision Date`, `Decision Type`, `Document Type`, `BS7666UPRN`, `Easting`, `Northing`.
    
- `land_charges_cards.csv` — 49,760 rows, columns include `Job Number`, `Filepath`, `Card Code`, `Address`, `BS7666UPRN`, `Easting`, `Northing`.
    
- `enforcement_notices.csv` — 1,172 rows, columns include `Job Number`, `Filepath`, `Planning Enforcement Reference Number`, `Address`, `Date`, `Document Type`, `BS7666UPRN`, `Easting`, `Northing`.
    
- `agreements.csv` — 2,602 rows, columns include `Job Number`, `Filepath`, `Address`, `Date`, `BS7666UPRN`, `Easting`, `Northing`.
    
- `ehdc_llpg_20250710.csv` — 71,904 rows, columns include `locaddress`, `easting`, `northing`, `bs7666uprn`, `bs7666usrn`, `blpuclass`, etc. **Important:** this LLPG extract does **not** contain a postcode column; the `postal` field is a Y/N flag, not a postcode.
    

### Missingness highlights (measured on your data)

- **Decision notices:** 90.2% missing UPRN/Easting/Northing; ~7.5% of rows have a blank `Adress`.
    
- **Land charges cards:** ~60.2% missing UPRN/Easting/Northing; address present in virtually all rows.
    
- **Enforcement notices:** ~92.5% missing UPRN/Easting/Northing; address present in ~99.6% rows.
    
- **Agreements:** ~78.0% missing UPRN/Easting/Northing; address present in all rows.
    

I also tested “what we can fill in immediately” using only:

1. rows whose **existing UPRN is valid and present in LLPG**, and
    
2. rows whose **normalised address exactly matches** the LLPG normalised address (postcode stripped, punctuation removed, spaces normalised).
    

Estimated “auto-fillable” (today, with the uploaded LLPG), by source:

- Decision notices: **18.95%** (7,456 by valid UPRN + 6,980 by canonical address match)
    
- Land charges cards: **42.29%** (10,559 + 10,486)
    
- Enforcement notices: **12.71%** (88 + 61)
    
- Agreements: **24.71%** (557 + 86)
    

> Note: In `land_charges_cards.csv`, **19,816 rows** carry a UPRN but only **10,559** of those UPRNs appear in the current LLPG. That means a significant tranche of ‘legacy’ or mis-typed UPRNs that need validation/fallback matching.

I’ve also measured postcode presence **within your source addresses** (regex-detected), which is helpful for candidate narrowing during fuzzy matching:

- Decision notices: 43.2%
    
- Land charges cards: 63.6%
    
- Enforcement notices: 75.7%
    
- Agreements: 37.6%
    

(LLPG doesn’t have a postcode column in this extract, so we’ll rely on address-string logic and/or optional enrichment.)

---

# Database specification (target schema)

Use **PostgreSQL + PostGIS + pg_trgm**. Names use `snake_case`. This schema separates **staging (raw)** from **core dimensions/facts** and **matching outcomes**. It’s designed so you can re-run, audit, and improve matching over time.

## 1) Extensions

`CREATE EXTENSION IF NOT EXISTS postgis; CREATE EXTENSION IF NOT EXISTS pg_trgm; CREATE EXTENSION IF NOT EXISTS unaccent;`

## 2) Staging tables (one-for-one with supplied CSVs)

Keep raw columns exactly as received.

	-- LLPG (authoritative)
	CREATE TABLE stg_llpg (
	  ogc_fid       bigint,
	  locaddress    text,
	  easting       numeric,
	  northing      numeric,
	  lgcstatusc    text,
	  bs7666uprn    text PRIMARY KEY,
	  bs7666usrn    text,
	  landparcel    text,
	  blpuclass     text,
	  postal        text
	);
	
	-- Decisions
	CREATE TABLE stg_decision_notices (
	  job_number                    text,
	  filepath                      text,
	  planning_application_number   text,
	  adress                        text,  -- as-is (typo preserved)
	  decision_date                 text,
	  decision_type                 text,
	  document_type                 text,
	  bs7666uprn                    text,
	  easting                       text,
	  northing                      text
	);
	
	-- Land charges
	CREATE TABLE stg_land_charges_cards (
	  job_number   text,
	  filepath     text,
	  card_code    text,
	  address      text,
	  bs7666uprn   text,
	  easting      text,
	  northing     text
	);
	
	-- Enforcement
	CREATE TABLE stg_enforcement_notices (
	  job_number  text,
	  filepath    text,
	  planning_enforcement_reference_number text,
	  address     text,
	  date        text,
	  document_type text,
	  bs7666uprn  text,
	  easting     text,
	  northing    text
	);
	
	-- Agreements
	CREATE TABLE stg_agreements (
	  job_number  text,
	  filepath    text,
	  address     text,
	  date        text,
	  bs7666uprn  text,
	  easting     text,
	  northing    text
	);


## 3) Core dimension tables

	-- Master address dimension from LLPG
	CREATE TABLE dim_address (
	  uprn            text PRIMARY KEY,
	  locaddress      text NOT NULL,
	  addr_can        text GENERATED ALWAYS AS (
	                    regexp_replace(
	                      regexp_replace(upper(locaddress),
	                        '\b([A-Z]{1,2}[0-9][0-9A-Z]?\s*[0-9][A-Z]{2})\b', '', 'g'  -- strip postcodes if any slipped in
	                      ),
	                      '[^A-Z0-9 ]', ' ', 'g'
	                    )::text
	                    -- collapse whitespace in a view
	                  ) STORED,
	  easting         numeric NOT NULL,
	  northing        numeric NOT NULL,
	  usrn            text,
	  blpu_class      text,
	  postal_flag     text,
	  geom27700       geometry(Point, 27700), -- OSGB36 / British National Grid
	  geom4326        geometry(Point, 4326)   -- WGS84 (lat/lon)
	);
	
	CREATE INDEX dim_address_addr_can_trgm_idx ON dim_address USING gin (addr_can gin_trgm_ops);
	CREATE INDEX dim_address_geom27700_idx    ON dim_address USING gist (geom27700);


## 4) Source “document” tables (normalised layer)

We’ll keep one “umbrella” table with a `source_type` to unify all four datasets. This makes matching logic and reporting simpler.

	CREATE TYPE source_type AS ENUM ('decision', 'land_charge', 'enforcement', 'agreement');
	
	CREATE TABLE src_document (
	  src_id          bigserial PRIMARY KEY,
	  source_type     source_type NOT NULL,
	  job_number      text,
	  filepath        text,
	  external_ref    text,      -- planning_application_number / card_code / enforcement_ref / etc
	  doc_type        text,
	  doc_date        date,
	  raw_address     text,      -- 'Adress' or 'Address' as-is
	  addr_can        text,      -- canonicalised at load (same method as dim_address)
	  postcode_text   text,      -- postcode extracted from raw_address if present
	  uprn_raw        text,      -- BS7666UPRN as provided (may be empty/invalid)
	  easting_raw     text,
	  northing_raw    text,
	  created_at      timestamptz DEFAULT now()
	);
	
	CREATE INDEX src_document_addr_can_trgm_idx ON src_document USING gin (addr_can gin_trgm_ops);
	CREATE INDEX src_document_source_type_idx   ON src_document (source_type);
	CREATE INDEX src_document_postcode_idx      ON src_document (postcode_text);


## 5) Matching results, audit & overrides

	-- One row per attempt; multiple attempts allowed per source row (algorithm evolution, reviews, etc.)
	CREATE TABLE match_run (
	  run_id          bigserial PRIMARY KEY,
	  run_started_at  timestamptz DEFAULT now(),
	  run_label       text,                -- e.g. "v1.0-initial"
	  notes           text
	);
	
	CREATE TABLE match_result (
	  match_id        bigserial PRIMARY KEY,
	  run_id          bigint REFERENCES match_run(run_id),
	  src_id          bigint REFERENCES src_document(src_id),
	  candidate_uprn  text REFERENCES dim_address(uprn),
	  method          text NOT NULL,       -- e.g. 'valid_uprn', 'addr_exact', 'trgm_0.88', 'manual'
	  score           numeric,             -- 0..1 similarity or confidence
	  tie_rank        int,                 -- 1 = best candidate in that attempt
	  decided         boolean DEFAULT false,
	  decision        text,                -- 'accepted'|'rejected'|'needs_review'
	  decided_by      text,                -- username / system
	  decided_at      timestamptz
	);
	
	-- Final accepted links (point src -> address via UPRN), separate for easy joins
	CREATE TABLE match_accepted (
	  src_id          bigint PRIMARY KEY REFERENCES src_document(src_id),
	  uprn            text REFERENCES dim_address(uprn),
	  method          text NOT NULL,
	  score           numeric,
	  run_id          bigint REFERENCES match_run(run_id),
	  accepted_by     text DEFAULT 'system',
	  accepted_at     timestamptz DEFAULT now()
	);
	
	-- Manual overrides (persist reviewers' decisions and corrections)
	CREATE TABLE match_override (
	  override_id     bigserial PRIMARY KEY,
	  src_id          bigint REFERENCES src_document(src_id),
	  uprn            text REFERENCES dim_address(uprn),
	  reason          text,
	  created_by      text,
	  created_at      timestamptz DEFAULT now()
);


## 6) Data quality support tables

	-- Normalisation rules (expandable)
	CREATE TABLE address_normalise_rule (
	  rule_id     bigserial PRIMARY KEY,
	  pattern     text,       -- regex or token (e.g., '\bRD\b')
	  replace_with text,      -- 'ROAD'
	  enabled     boolean DEFAULT true,
	  weight      int DEFAULT 0
	);
	
	-- Common UK street abbreviations (seed with Rd->Road, St->Street, Ave->Avenue, Gdns->Gardens, etc.)


---

# ETL & matching process (very detailed requirements)

## A. Ingestion & normalisation

1. **Load LLPG** (`stg_llpg`) and build `dim_address`:
    
    - Copy all rows.
        
    - Treat `bs7666uprn` as **text** primary key (UPRNs can be up to 12 digits; keep as text to avoid leading-zero/format issues).
        
    - **Create canonical address** `addr_can` = `LOCADDRESS` uppercased, punctuation removed, **postcode stripped** (if any), whitespace collapsed.
        
    - Cast `easting`/`northing` to numeric.
        
    - Populate geometry columns: `geom27700` from (Easting, Northing) in EPSG:27700, and `geom4326` via `ST_Transform`.
        
2. **Load the four source files** into `src_document`:
    
    - Map columns to a unified schema:
        
        - `source_type`: ‘decision’, ‘land_charge’, ‘enforcement’, or ‘agreement’.
            
        - `external_ref`: use the most relevant reference per source (e.g., `Planning Application Number` or `Planning Enforcement Reference Number`).
            
        - `raw_address`: `Adress` (decision) / `Address` (others).
            
        - `doc_date`: parse if present; else NULL.
            
        - `uprn_raw`, `easting_raw`, `northing_raw` from the columns if present.
            
    - **Canonicalise addresses** to `addr_can` using the same function as LLPG (strip postcodes, normalise punctuation/whitespace).
        
    - **Extract postcode** from `raw_address` via UK postcode regex and store in `postcode_text`.
        
    - Trim and store `uprn_raw` exactly (do not coerce to numeric).
        
    - Store the file path and job number for traceability.
        

**Notes on normalisation:**

- Token-expansion (Street→STREET, Rd→ROAD, Ave→AVENUE, etc.) will be applied _before_ canonicalisation; store updated `addr_can`.
    
- Retain the original `raw_address` verbatim for audit/troubleshooting.
    
- Maintain a rule table (`address_normalise_rule`) for future tweaks without redeploying code.
    

## B. Deterministic matching (Phase 1)

Run ID: create a `match_run` row (e.g., label `v1.0-initial`).

1. **Valid UPRN match:**  
    If `uprn_raw` is present and exists in `dim_address.uprn`, create `match_result` with:
    
    - `method = 'valid_uprn'`
        
    - `score = 1.0`
        
    - `tie_rank = 1`
        
    - Immediately write to `match_accepted` (system-decided) and mark as `decided=true, decision='accepted'`.
        
2. **Canonical exact address match (for rows not already matched):**  
    Join `src_document.addr_can` to `dim_address.addr_can`.
    
    - `method = 'addr_exact'`
        
    - `score = 0.99`
        
    - If exactly one candidate → accept directly.
        
    - If multiple candidates → write all into `match_result` with appropriate `tie_rank`, `decided=false`, `decision='needs_review'`.
        

> With your current data this alone gets roughly the “auto-fill potential” percentages listed above.

## C. Probabilistic matching (Phase 2 – fuzzy, staged)

Use **pg_trgm** (fast and robust). All fuzzy matches should:

- Set an **explicit similarity score** (`similarity(src.addr_can, dim.addr_can)`).
    
- Record **top N candidates** (suggest N=3 per source row where similarity ≥ threshold).
    
- Apply **staged thresholds**:
    
    - **Tier 1 (high confidence):** `similarity ≥ 0.90` → Accept automatically _if single best candidate_ and next-best < 0.02 lower. Method label: `trgm_0.90`.
        
    - **Tier 2 (medium):** `0.85 ≤ similarity < 0.90` → Auto-accept only if candidate is unique (≥ 0.03 gap to next). Else queue for review. Method label: `trgm_0.85`.
        
    - **Tier 3 (low):** `0.80 ≤ similarity < 0.85` → Always queue for review. Method label: `trgm_0.80`.
        

**Candidate narrowing** (to avoid false positives and improve speed):

- Prefer candidates whose `addr_can` contains the **same locality/post town tokens** found in `raw_address` (e.g., ALTON, PETERSFIELD, LIPHOOK, WATERLOOVILLE, etc.). Extract a small set of town/locality tokens from `raw_address` (uppercase words not in a stoplist such as “ROAD”, “LANE”, “HOUSE”, etc.) and require at least one token overlap.
    
- If your source address includes a **house number** or **flat number**, require the number token to be present in the candidate `addr_can`. (We can parse tokens like `12`, `12A`, `FLAT 2`, `APARTMENT 3`, etc.)
    

**Example fuzzy SQL sketch:**

`-- For unmatched src rows WITH unmatched AS (   SELECT s.src_id, s.addr_can   FROM src_document s   LEFT JOIN match_accepted m ON m.src_id = s.src_id   WHERE m.src_id IS NULL ) INSERT INTO match_result (run_id, src_id, candidate_uprn, method, score, tie_rank) SELECT :run_id, u.src_id, d.uprn, 'trgm_0.90' AS method,        similarity(u.addr_can, d.addr_can) AS score,        ROW_NUMBER() OVER (PARTITION BY u.src_id ORDER BY similarity(u.addr_can, d.addr_can) DESC) AS tie_rank FROM unmatched u JOIN dim_address d   ON u.addr_can %% d.addr_can           -- trigram candidate operator WHERE similarity(u.addr_can, d.addr_can) >= 0.90 ORDER BY u.src_id, score DESC;`

Then decide/accept according to the rules above (unique, gap to next, etc.).

## D. Review workflow (Phase 3 – human-in-the-loop)

For any `match_result` rows with `decided=false`:

- Export a review sheet (CSV or a simple UI) containing:
    
    - `src_id`, `source_type`, `external_ref`, `raw_address`, `postcode_text`
        
    - Top 3 candidate `uprn`, `locaddress`, `easting`, `northing`, `score`
        
    - One-click “Accept” for a candidate or “Reject all”.
        
- On accept, insert/update `match_accepted` and mark the chosen `match_result` with `decided=true/accepted`. If all are rejected, set `decision='rejected'` and optionally create a `match_override` later.
    

## E. Back-fill coordinates

Once a `src_document` has an accepted UPRN link:

- Update a materialised view (or run-time join) to surface **authoritative** `easting`/`northing` from LLPG.
    
- Keep original raw coordinates (if any) for comparison, but treat LLPG as canonical.
    

## F. Optional enrichment (recommended but not mandatory)

Because the LLPG extract lacks postcodes, consider **linking LLPG to ONSPD** (Office for National Statistics Postcode Directory) or AddressBase (licensed) to lookup postcodes by BNG coordinates. If added:

- Store `postcode` in `dim_address` and **tighten deterministic rules** (e.g., postcode + house number exact).
    
- Use postcode to narrow fuzzy candidates drastically.
    

---

# Data cleaning rules (initial set)

- **Whitespace & punctuation:** collapse repeated whitespace; remove commas, periods, parentheses except those in flat/unit numbers where possible.
    
- **Abbreviation expansion:** `RD`→ROAD, `ST`→STREET (but keep `SAINT` context separate), `AVE`→AVENUE, `GDNS`→GARDENS, `PK`→PARK, `CT`→COURT, `DR`→DRIVE, `LN`→LANE, `PL`→PLACE, `SQ`→SQUARE, `CRES`→CRESCENT, `TER`→TERRACE, `CL`→CLOSE.
    
- **Ordinal/alpha unit handling:** normalise `12A` and `FLAT 2` as tokens; keep both number and alpha suffix.
    
- **Postcode handling:** strip postcodes from canonical form but store them separately in `postcode_text`.
    
- **Case:** uppercase for canonical matching.
    
- **Diacritics:** use `unaccent()` in PostgreSQL to remove accents.
    

All rules are maintained in `address_normalise_rule` so we can enhance without code changes.

---

# Outputs & reconciliation

1. **Updated link table**: `match_accepted` (one row per document → UPRN).
    
2. **Completion metrics** (by source and overall):
    
    - % matched by `valid_uprn`
        
    - % matched by `addr_exact`
        
    - % matched by `trgm_0.90`, `trgm_0.85`, `trgm_0.80`
        
    - % requiring review; % unresolved
        
3. **Export feeds** (if needed):
    
    - Per-source CSV with appended `UPRN`, `Easting`, `Northing`, `matched_by`, `match_score`, `run_label`.
        
4. **Exception logs**:
    
    - Rows with invalid legacy UPRNs.
        
    - Rows with multiple high-scoring candidates (ambiguity).
        
    - Rows with addresses missing critical tokens (e.g., completely blank).
        

---

# Acceptance criteria

- **Schema:** All tables created, extensions enabled (PostGIS, pg_trgm, unaccent).
    
- **Load:** 100% of provided rows present in `src_document`; LLPG fully populated in `dim_address`.
    
- **Coordinates:** For every accepted match, `Easting`/`Northing` sourced from LLPG and exposed via a view or export.
    
- **Auditability:** Every automated decision has a `match_run`, `method`, `score`, and (when relevant) alternative candidates recorded.
    
- **Deterministic coverage:** At least the auto-fill levels measured above are achieved by Phase 1 (your current data already supports these).
    
- **Fuzzy coverage:** Additional uplift achieved with trigram matching (targets set collaboratively after a calibration run, with precision > 98% on Tier 1 auto-accepts, and zero false auto-links found in spot checks).
    
- **Review queue:** All ambiguous cases available in a simple reviewer export (or UI) with ability to accept/reject and record overrides.
    
- **No data loss:** Original raw addresses and raw coordinates remain intact for traceability.
    

---

# Implementation notes (Golang-first, Postgres-centred)

Given your stack and debugging conventions:

**Tech choices**

- **PostgreSQL 15+**, **PostGIS**, **pg_trgm**, **unaccent**.
    
- **Go** ETL app using:
    
    - `lib/pq` or `pgx` for Postgres.
        
    - Your `debug` package everywhere (Header/Output/Footer pattern).
        
    - A small normalisation pkg (pure-Go) mirroring the SQL rules to precompute `addr_can` on ingest (keeps SQL in sync, still re-check in DB).
        
- **Makefile / Docker Compose**: add Postgres service (if not present), mount SQL migrations, run ETL container(s).
    

**Suggested Go package layout**

	cmd/uplift/main.go          # CLI: ingest, match, export
	internal/db/                # migrations, queries
	internal/normalize/         # address canonicaliser & postcode extractor
	internal/match/             # deterministic & fuzzy orchestrators
	internal/report/            # metrics, CSV exports


**Key functions** (all with your debug pattern)

- `normalize.Canonical(raw string) (addrCan string, postcode string, tokens []string)`
    
- `ingest.LoadLLPG(csv string)`
    
- `ingest.LoadSource(csv string, sourceType string)`
    
- `match.RunDeterministic(runLabel string)` → writes `match_result`, `match_accepted`
    
- `match.RunFuzzy(runLabel string, tier float64)` → writes candidates; performs auto-accepts per thresholds
    
- `report.Summary(runLabel string)` → prints coverage metrics; writes CSVs
    

**SQL helpers**

- Migrations with the DDL above (recommend `golang-migrate/migrate`).
    
- Prepared statements for batched inserts.
    
- Index maintenance and `ANALYZE` post-load.
    

---

# Risks & mitigations

- **Legacy/invalid UPRNs in source** (observed in `land_charges_cards`): validate against LLPG; fallback to address matching.
    
- **Ambiguous multi-candidate matches**: do not auto-accept; queue for review with clear scoring.
    
- **Inconsistent abbreviations & ordering**: handled by normalisation and trigram matching; tune rules iteratively via the rules table.
    
- **LLPG lacking postcodes**: consider optional ONSPD/AddressBase enrichment to add postcodes by coordinate, improving precision and explainability.
    
- **Performance**: use GIN trigram index on `addr_can`, locality token pre-filters, and process in batches.
    

---

# What you get at the end

- A **clean, auditable link** from every source document to an **authoritative UPRN** (or a review task if ambiguous).
    
- **Easting/Northing** filled from LLPG for all accepted matches.
    
- **Metrics** that demonstrate uplift and show exactly how each link was made.
    
- A **repeatable pipeline** you can rerun when LLPG or source data updates arrive, preserving prior manual decisions.