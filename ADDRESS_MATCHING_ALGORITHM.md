Below is a battle-tested, “use-everything” design for a **Go-based expert address matcher** that suggests the most likely **modern UPRN** from historic/dirty addresses. It mixes deterministic rules, statistical and vector matching, geospatial proximity, phonetics, and active-learning with human review—while staying local-first and free.

I’ll give you: architecture, data flow, algorithms, thresholds, model choices, and **Go code skeletons** (with your `debug`hooks) you can drop into a repo.

---

# 1) Goals

- Given an old/open-text address (and optional legacy UPRN/E/N), return **ranked UPRN candidates** with **explainable reasons**.
    
- Achieve high precision on auto-accepts; route ambiguous cases to review; learn from corrections.
    
- Run locally (no paid APIs). Be modular so you can add/replace components (LLMs, embeddings, stores).
    

---

# 2) Architecture (local-first & modular)

**Core stores**

- **PostgreSQL + PostGIS + pg_trgm + unaccent**
    
    - `dim_address` (LLPG) with `uprn`, `locaddress`, `easting`, `northing`, `geom27700`, `geom4326`, `addr_can`(canonical address).
        
    - `src_document` (historic sources) with `raw_address`, `addr_can`, `postcode_text`, optional `easting_raw`, `northing_raw`, `uprn_raw`.
        
    - `match_result`, `match_accepted`, `match_override`, `match_run` (audit + learning).
        
- **Vector DB**: **Qdrant** (free, Docker) using **HNSW** for fast ANN on address embeddings.
    
- **Sidecars / libraries**
    
    - **libpostal** for parsing & normalisation (C library).
        
    - **Ollama** with free local **embedding model** (e.g. `nomic-embed-text`, `bge-small-en`, or `all-minilm-l6-v2`via ONNX/TEI).
        
    - **Double Metaphone** in Go for phonetic matching.
        
    - Optional: **self-hosted Nominatim** only if you want postcode enrich/reverse-geo (respect usage policy), but not required.
        

**High-level flow (per query)**

      ┌────────────────────┐
      │   Input address    │ (raw historic text + optional E/N, date, legacy UPRN)
      └─────────┬──────────┘
                │ normalise + tokenise (libpostal + rules)
                ▼
        ┌──────────────┐
        │ Candidate     │  A) deterministic (valid UPRN, exact addr_can)
        │ generation    │  B) trigram/phonetic/USRN-limited
        │ (wide)        │  C) vector ANN (Qdrant)
        └──────┬────────┘
               ▼
        ┌──────────────┐
        │ Feature set   │  string sims, phonetics, token overlap, embeddings,
        │ computation   │  locality hits, number matches, E/N distance etc.
        └──────┬────────┘
               ▼
        ┌──────────────┐
        │ Rank/score    │  calibrated meta-score → Top-N UPRNs
        └──────┬────────┘
               ▼
        ┌──────────────┐
        │ Decision      │  thresholds: auto-accept / review / reject
        └──────────────┘


---

# 3) Normalisation & tokenisation (UK-centric)

**Tools**

- `libpostal` parse + expand.
    
- Custom rules table for UK abbreviations, heritage forms, “LAND AT/ADJACENT TO/REAR OF” etc.
    
- `unaccent()` (DB) to drop diacritics.
    

**Canonical form (`addr_can`)**

- Uppercase
    
- Remove postcode (store in `postcode_text`)
    
- Expand common abbreviations: `RD→ROAD`, `ST→STREET (but SAINT stays)`, `AVE→AVENUE`, `GDNS→GARDENS`, `CT→COURT`, `DR→DRIVE`, `LN→LANE`, `PL→PLACE`, `SQ→SQUARE`, `CRES→CRESCENT`, `TER→TERRACE`, `CL→CLOSE`
    
- Remove punctuation, collapse whitespace
    
- Keep **house/flat numbers and alpha suffixes** (e.g., `12A`)
    
- Keep **locality/town tokens** (ALTON, PETERSFIELD, LIPHOOK, WATERLOOVILLE, etc.)
    

**Phonetics**

- Compute **Double Metaphone** for each street/locality token to catch misspellings (HORNDENE vs HORNDEAN).
    

**Numbers**

- Extract: PAON/SAON candidates (e.g., `FLAT 2`, `APT 3`, `PLOT 61`, `UNIT 7A`).
    

---

# 4) Candidate generation (wide net)

**Tier A – Deterministic**

1. **Legacy UPRN validation**: if present and in LLPG → candidate with score=1.0 seed.
    
2. **Exact canonical match**: `src.addr_can = dim.addr_can` → strong candidate(s).
    

**Tier B – Database fuzzy**  
3) **Trigram (pg_trgm)**: `src.addr_can %% dim.addr_can`

- Start with `similarity >= 0.80` (collect top 50).
    

4. **Phonetic filter**: require at least one phonetic match in core tokens (street/locality) to remain candidate.
    
5. **Locality filter**: if we detect town/locality tokens in source, require overlap.
    
6. **House number filter**: if a number is present, prefer candidates containing same number token (or number±1 for renumbering heuristics).
    
7. **USRN proximity** (if you load OS Open USRN or use `bs7666usrn` from LLPG): prefer same USRN.
    

**Tier C – Vector ANN (semantic)**  
8) **Embeddings**: embed `src.addr_can` and query **Qdrant** over LLPG `addr_can` vectors.

- Keep top K (e.g., 50) with cosine similarity.
    
- Union with DB fuzzy candidates (dedupe by UPRN).
    

**Tier D – Spatial hints (if available)**  
9) If source has any E/N (even rough), compute distance to candidates’ LLPG points; keep within radius **R** (e.g., 2 km default; wider if rural/locality wide).

> The generator returns a **union** of up to a few hundred candidates, but usually dozens after filters.

---

# 5) Features per (source, candidate)

Compute a rich feature vector `F`:

- **String similarities**:
    
    - `trgm_sim`: PostgreSQL similarity
        
    - `jaro`, `levenshtein_norm` (normalised)
        
    - `cosine_bow` on token sets
        
- **Embeddings**:
    
    - `embed_cos`: cosine similarity from vector model
        
- **Token/structure features**:
    
    - `has_same_house_num` (bool)
        
    - `has_same_house_alpha` (bool, e.g., `12A`)
        
    - `locality_overlap_ratio` (#overlapping locality tokens / #locality tokens in src)
        
    - `street_overlap_ratio` on street tokens (normalised)
        
    - `descriptor_penalty` (e.g., source contains `REAR OF`, `LAND AT`; candidate lacks parcel indicator)
        
- **Phonetics**:
    
    - `phonetic_hits` (count of token phonetic matches)
        
- **Spatial** (if src E/N present):
    
    - `dist_m` (metres), `dist_bucket` (0–100m, 100–250m, 250–500m, …)
        
    - `spatial_boost = exp(-dist_m / 300)` (tunable)
        
- **Meta**:
    
    - `llpg_status` (e.g., `lgcstatusc == 1` → boost live addresses)
        
    - `blpu_class_compat` (residential vs non-res; school/farm hints found in text)
        
    - `usrn_match` (exact/similar)
        
- **Date-aware (optional)**:
    
    - If document date is far in the past and site redeveloped, prefer **current** address in same parcel/street match. (We can’t infer demolitions from your current LLPG alone, but we can down-weight retired `lgcstatusc` if available.)
        

---

# 6) Meta-scoring & decisions

**Scoring formula (logistic blend; calibrated later)**

Start with a transparent baseline (weights can be learned later by logistic regression/GBM; numbers below are sane defaults):
	
	score_raw =
	  0.45 * trgm_sim +
	  0.45 * embed_cos +
	  0.05 * locality_overlap_ratio +
	  0.05 * street_overlap_ratio
	  + 0.08  if has_same_house_num
	  + 0.02  if has_same_house_alpha
	  + 0.04  if usrn_match
	  + 0.03  if llpg_status_is_live
	  + spatial_boost_term
	  - 0.05  if descriptor_penalty_applies
	  - 0.03  if phonetic_hits == 0
	  + 0.20  if legacy_uprn_valid (cap at 1.0 after blending)


Clamp `score = min(1.0, max(0.0, score_raw))`.

**Decision thresholds (tune with your gold set)**

- **Auto-accept** if:
    
    - `score ≥ 0.92` **and** margin to next candidate ≥ `0.03`, **or**
        
    - `score ≥ 0.88` with same-house number and locality overlap ≥ 0.5 and next < `score - 0.05`.
        
- **Manual review** if `0.80 ≤ score < auto_threshold` or close ties.
    
- **Reject** if `score < 0.80`.
    

All calculations and the exact factors for each candidate are recorded for audit/explainability.

---

# 7) AI & free components (concrete picks)

- **Embeddings (local):**
    
    - **Ollama** with `nomic-embed-text` or `bge-small-en`.
        
    - Alternative: **Hugging Face Text Embeddings Inference (TEI)** with `all-MiniLM-L6-v2` (ONNX), run locally in Docker; call via HTTP from Go.
        
- **Vector DB:** **Qdrant** (Docker) with HNSW; Go client available.
    
- **Parser/normaliser:** **libpostal** (C) + a thin Go wrapper or a sidecar HTTP microservice (easiest).
    
- **Phonetics:** Go Double Metaphone lib (e.g., `github.com/dotcypress/phonetics`).
    
- **Database fuzzy:** `pg_trgm`, `unaccent`, PostGIS for distance.
    
- **Optional enrichment:**
    
    - **OS Boundary-Line** (free) to detect district/parish from candidate point and prefer same area if the source mentions it.
        
    - **Self-hosted Nominatim** only if you decide to infer postcodes for LLPG by reverse-geo; not required for UPRN suggestion.
        

---

# 8) Go interfaces & skeletons (with your `debug` package)

## 8.1 Types

	package match
	
	type Input struct {
	    RawAddress   string
	    Easting      *float64 // optional
	    Northing     *float64 // optional
	    LegacyUPRN   string   // optional
	    SourceType   string   // e.g. "decision", "enforcement"
	    DocDate      *time.Time
	}
	
	type Candidate struct {
	    UPRN        string
	    LocAddress  string
	    Easting     float64
	    Northing    float64
	    Score       float64
	    Features    map[string]any // explainability
	    Methods     []string       // which generators hit (valid_uprn, trigram, vector, etc.)
	}
	
	type Result struct {
	    Query         Input
	    Candidates    []Candidate // sorted hi→lo
	    Decision      string      // "auto_accept" | "review" | "reject"
	    AcceptedUPRN  string
	    Thresholds    map[string]float64
	}


## 8.2 Normaliser

	package normalize
	
	import (
	    "regexp"
	    "strings"
	    "unicode"
	    "github.com/yourorg/debug"
	)
	
	var rePostcode = regexp.MustCompile(`\b([A-Za-z]{1,2}\d[\dA-Za-z]?\s*\d[ABD-HJLNP-UW-Zabd-hjlnp-uw-z]{2})\b`)
	
	func CanonicalAddress(local_debug bool, raw string, rules AbbrevRules) (addrCan, postcode string, tokens []string) {
	    debug.DebugHeader(local_debug)
	    defer debug.DebugFooter(local_debug)
	
	    s := strings.ToUpper(strings.TrimSpace(raw))
	    // extract postcode
	    if m := rePostcode.FindString(s); m != "" {
	        postcode = strings.ReplaceAll(m, " ", "")
	        s = rePostcode.ReplaceAllString(s, " ")
	    }
	    // remove punctuation
	    b := strings.Builder{}
	    for _, r := range s {
	        if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
	            b.WriteRune(r)
	        } else {
	            b.WriteRune(' ')
	        }
	    }
	    s = strings.Join(strings.Fields(b.String()), " ")
	
	    // expand abbreviations (rules sourced from DB)
	    s = rules.Expand(s)
	
	    // collapse spaces again
	    s = strings.Join(strings.Fields(s), " ")
	
	    tokens = strings.Fields(s)
	    debug.DebugOutput(local_debug, "addr_can: %s", s)
	    return s, postcode, tokens
	}


## 8.3 Candidate generators

	package generator
	
	type Generators struct {
	    PG  *sql.DB          // pg_trgm + postgis
	    VDB QdrantClient     // vector DB
	    Emb Embedder         // Ollama/TEI client
	    Libpostal Parser     // sidecar or cgo wrapper
	}
	
	func (g *Generators) Generate(local_debug bool, in match.Input, can string, toks []string) ([]match.Candidate, error) {
	    debug.DebugHeader(local_debug); defer debug.DebugFooter(local_debug)
	
	    var out []match.Candidate
	
	    // A) Legacy UPRN
	    if in.LegacyUPRN != "" {
	        if cand, ok := g.lookupUPRN(in.LegacyUPRN); ok {
	            cand.Methods = append(cand.Methods, "legacy_uprn_valid")
	            cand.Features = map[string]any{"legacy_upn_hit": true}
	            out = append(out, cand)
	        }
	    }
	
	    // B) Exact canonical match
	    exact := g.pgExact(local_debug, can)
	    out = append(out, exact...)
	
	    // C) Trigram fuzzy (pre-filter by locality tokens)
	    trgm := g.pgTrigram(local_debug, can, toks)
	    out = append(out, trgm...)
	
	    // D) Vector ANN via Qdrant (embed the canonical string once)
	    vec, err := g.Emb.Embed(can)
	    if err == nil {
	        vCands := g.vdbQuery(local_debug, vec)
	        out = append(out, vCands...)
	    }
	
	    // E) Spatial narrowing if input has E/N
	    if in.Easting != nil && in.Northing != nil {
	        out = g.spatialFilter(local_debug, out, *in.Easting, *in.Northing, 2000.0) // 2km
	    }
	
	    // Dedupe by UPRN (keep highest preliminary score or union features)
	    out = dedupeByUPRN(out)
	
	    return out, nil
	}


### Trigram SQL (example)

	-- GIN index (once)
	CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE INDEX dim_address_addr_can_trgm_idx ON dim_address USING gin (addr_can gin_trgm_ops);
	
	-- Query pattern (Go prepares and binds):
	SELECT uprn, locaddress, easting::float8, northing::float8, similarity($1, addr_can) AS trgm
	FROM dim_address
	WHERE addr_can %% $1
	ORDER BY trgm DESC
	LIMIT 200;


## 8.4 Feature computation & scoring

	package rank
	
	func ComputeFeatures(local_debug bool, in match.Input, can string, toks []string, cand match.Candidate) map[string]any {
	    debug.DebugHeader(local_debug); defer debug.DebugFooter(local_debug)
	    f := map[string]any{}
	
	    // trigram already attached by generator; add others:
	    f["jaro"] = Jaro(can, cand.LocAddress) // use same canonicalisation for target
	    f["lev"]  = LevenshteinNorm(can, cand.LocAddress)
	    f["token_locality_overlap"] = LocalityOverlap(toks, cand.LocAddress)
	    f["token_street_overlap"]   = StreetOverlap(toks, cand.LocAddress)
	    f["has_same_house_num"]     = SameHouseNumber(toks, cand.LocAddress)
	    f["has_same_house_alpha"]   = SameHouseAlpha(toks, cand.LocAddress)
	    f["usrn_match"]             = USRNMatch(in, cand) // if available
	    f["llpg_live"]              = true // from llpg status if loaded
	
	    // spatial
	    if in.Easting != nil && in.Northing != nil {
	        d := DistMeters27700(*in.Easting, *in.Northing, cand.Easting, cand.Northing)
	        f["dist_m"] = d
	        f["spatial_boost"] = math.Exp(-d / 300.0)
	    } else {
	        f["spatial_boost"] = 0.0
	    }
	    return f
	}
	
	func Score(local_debug bool, f map[string]any, legacyUPRNValid bool) float64 {
	    debug.DebugHeader(local_debug); defer debug.DebugFooter(local_debug)
	    s := 0.0
	
	    trgm, _ := asFloat(f["trgm"])
	    emb,  _ := asFloat(f["embed_cos"])
	    s += 0.45*trgm + 0.45*emb
	
	    s += 0.05*asFloat(f["token_locality_overlap"])
	    s += 0.05*asFloat(f["token_street_overlap"])
	
	    if asBool(f["has_same_house_num"])   { s += 0.08 }
	    if asBool(f["has_same_house_alpha"]) { s += 0.02 }
	    if asBool(f["usrn_match"])           { s += 0.04 }
	    if asBool(f["llpg_live"])            { s += 0.03 }
	
	    s += asFloat(f["spatial_boost"])
	
	    if legacyUPRNValid { s += 0.20 }
	
	    // penalties
	    if asBool(f["descriptor_penalty"]) { s -= 0.05 }
	
	    if s < 0 { s = 0 }
	    if s > 1 { s = 1 }
	    return s
	}


## 8.5 End-to-end suggestion

	package engine
	
	func SuggestUPRN(local_debug bool, in match.Input, gens *generator.Generators, sc *rank.Scorer) (match.Result, error) {
	    debug.DebugHeader(local_debug); defer debug.DebugFooter(local_debug)
	
	    // 1) Normalise
	    addrCan, _, toks := normalize.CanonicalAddress(local_debug, in.RawAddress, normalize.DefaultRules())
	
	    // 2) Generate candidates
	    cands, err := gens.Generate(local_debug, in, addrCan, toks)
	    if err != nil { return match.Result{}, err }
	
	    // 3) Compute features & embed cosine
	    //    Ensure embed_cos present (if vector stage didn’t, do one-off dot products)
	    vec, _ := gens.Emb.Embed(addrCan)
	
	    for i := range cands {
	        feats := rank.ComputeFeatures(local_debug, in, addrCan, toks, cands[i])
	        if _, ok := feats["embed_cos"]; !ok {
	            feats["embed_cos"] = Cosine(vec, gens.VDB.VectorForUPRN(cands[i].UPRN))
	        }
	        cands[i].Features = feats
	        cands[i].Score = rank.Score(local_debug, feats, hasValidLegacy(in.LegacyUPRN))
	    }
	
	    sort.Slice(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	
	    // 4) Decision
	    decision, accepted := decide(cands)
	    res := match.Result{
	        Query:        in,
	        Candidates:   cands,
	        Decision:     decision,
	        AcceptedUPRN: accepted,
	        Thresholds:   map[string]float64{"auto": 0.92, "review": 0.80},
	    }
	    return res, nil
	}


---

# 9) Training & calibration (free, pragmatic)

- **Start with defaults** above.
    
- Build a **gold set** from rows where you already trust the mapping (e.g., valid UPRN rows; manual reviews).
    
- Fit a **logistic regression** on the feature set to learn weights: do this in Python (scikit-learn), export to **ONNX**, load it in Go via `onnxruntime-go` for production scoring.
    
- Re-train periodically as reviewers make decisions (active learning).
    

---

# 10) Docker Compose (local stack)

- **Postgres + PostGIS**
    
- **Qdrant**
    
- **Ollama** (embedding model)
    
- **libpostal sidecar** (HTTP microservice)
    
- **Matcher service** (Go)
    

_(Compose omitted here for brevity, but straightforward: three containers plus your Go app.)_

---

# 11) Data you already have (how it plugs in)

- **LLPG** → `dim_address` (uprn, locaddress, easting, northing, usrn, status).
    
- Pre-index: `pg_trgm` on `addr_can`; `gist` on `geom27700`; HNSW in Qdrant on embeddings of `addr_can`.
    
- **Historic sources** → `src_document` provide raw addresses for testing & to bootstrap the training set.
    

---

# 12) Beyond the basics (thinking outside the box)

- **Street renaming dictionary**: seed from council notices or OS Open USRN change logs; apply as a transform layer (e.g., “EASTLAND GATE” older alias recognised).
    
- **Descriptor handling**: recognise “LAND ADJACENT TO”, “REAR OF”, “PLOT ##” and re-weight to parcel-level candidates (same street, nearest numbers).
    
- **Multi-evidence boosting**: If the same UPRN appears as top-3 for **several documents in the same box/folder**(`Filepath`), increase its prior for near-duplicate addresses.
    
- **Temporal priors**: If a document is from year Y, prefer UPRNs with BLPU class consistent with that era (e.g., farms vs new estates), or with USRN present in that period (if you ingest USRN history later).
    
- **Gazetteer hints**: OS Boundary-Line parish/ward polygons let you boost candidates in the parish named in the address (“HAWKLEY”, “FOUR MARKS”).
    
- **LLM rewrite (optional, local)**: Pass the raw text to a tiny local LLM with a prompt that **rewrites** it into a clean UK address and extracts structured fields (PAON/SAON/street/locality). Use only as an extra generator to propose variants—not the final decision maker.
    

---

# 13) KPIs & acceptance

- **Auto-accept precision ≥ 98%** in spot checks.
    
- **Coverage uplift**: measure % with accepted UPRN per source; report breakdown by method (legacy, exact, trigram, vector, hybrid).
    
- **Explainability**: every suggestion shows features, scores, and reasons.
    
- **Latency**: ≤ 150 ms for DB fuzzy; ≤ 200 ms for vector ANN; end-to-end ≤ 600 ms per query on modest hardware.
    
- **Reproducibility**: versioned `match_run`, deterministic seed for embedding batch builds.
    

---

# 14) Rollout plan

1. Implement normaliser + deterministic stages; test on your datasets.
    
2. Add pg_trgm fuzzy + phonetics + locality/house filters.
    
3. Stand up Ollama + Qdrant; index LLPG embeddings; add vector stage.
    
4. Blend scores; tune thresholds with a small gold set.
    
5. Wire review export/import; capture overrides into `match_accepted` & re-train weights.
    
6. Add optional enrichments (Boundary-Line, USRN history) if needed.