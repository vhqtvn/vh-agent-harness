<!--
  PROVENANCE: read-only verification of public upstream sources, gathered
  2026-08-10. NO accuracy benchmarks were run (no corpus assumed). Every claim
  cites the exact URL/file read; where a doc was silent, it is stated as
  "not documented" rather than inferred.
  CLAIM CLASSES:
    - Class A (upstream primary): official repo README/LICENSE/raw files read via
      github raw + GitHub REST API; HuggingFace model pages; PyPI; crates.io;
      Mathpix official pricing/docs pages. Re-derivable by any reader.
    - Class B (could not directly load): GROBID readthedocs (persistent HTTP 429
      rate-limit) and one GROBID config path (404). Flagged inline; the GROBID
      default-service-port detail is therefore asserted-from-README only, not
      cited from the service doc.
    <!-- UPDATE: GROBID Class B items are RESOLVED by the addendum at the end of this file (port 8070/8071, endpoints, cadence). -->
  TIME-SENSITIVE ITEMS (re-derive before relying):
    - Mathpix per-page pricing (fetched 2026-08-10)
    - pdf-inspector PyPI + crates.io published versions (both 2026-07-31)
    - Nougat / UReader maintenance staleness (last-push dates)
  ARTIFACT TYPE: source packet (researches/sources/). Not a decision memo.
  DOWNSTREAM: intended to feed a debate/planner pass that turns these verified
  facts into architecture options for an extensible scientific-paper-reading
  pipeline. No active repo policy is changed by this file.
  STAGING NOTE: researcher session is read-only to researches/ (edits denied
  except tmp/**). This copy lives in tmp/; a write-capable session should commit
  it verbatim to researches/sources/2026-08-10-paper-pipeline-extractor-tool-facts.md
-->

---

# Scientific-paper-reading pipeline: verified extractor tool facts

**Researcher:** vh-agent-harness `researcher` session (read-only).
**Date:** 2026-08-10.
**Question:** For 7 candidate extractors (GROBID, Marker, MinerU, Mathpix,
Nougat, UReader, pdf-inspector), convert each tool's "claimed" deployment /
license / capability / maintenance posture into ground-truth facts verified from
the tool's own source. Close the evidence gaps a prior solution-brief flagged.
**Source policy:** primary only — official repo README/LICENSE/raw files, GitHub
REST API, HuggingFace model pages, PyPI, crates.io, official docs/pricing. No
secondary writeups, no accuracy benchmarks.
**Scope fences:** deployment reality, license reality (code + weights),
documented capability reality, maintenance signal. NOT in scope: head-to-head
accuracy benchmarks, integration code, cost modeling beyond cited list prices.

---

## TL;DR — verdicts on the brief's "claimed, unverified" items

| # | Brief's flagged item | Verdict | Basis |
|---|----------------------|---------|-------|
| 1 | GROBID canonical org/repo | **Confirmed: `grobidOrg/grobid`** (user's `krobidOrg` was a typo). `kermitt2` owns auxiliary repos only. | GitHub API `full_name` |
| 2 | GROBID bounding boxes / coordinates | **Confirmed.** README: "PDF coordinates for extracted information ... bounding boxes". | README |
| 3 | GROBID GPU optional / default CPU | **Confirmed.** Default = CRF/CPU; "NVIDIA GPU ... Optional". | README |
| 4 | Marker selective OCR / page-level repair | **Confirmed (documented at page AND block granularity).** | README |
| 5 | Marker/Surya weight license | **Confirmed NOT free-for-all.** Modified AI-Pubs OpenRAIL-M; commercial free only <$5M funding/revenue. | Marker + Surya README |
| 6 | MinerU CPU/GPU parity | **Refuted as "parity" — they are DIFFERENT pipelines.** CPU `pipeline` (86.47 acc) ≠ GPU `hybrid`/`vlm` (~95.3). | MinerU README backend table |
| 7 | MinerU custom Apache-derived license | **Confirmed.** Attribution + 100M-MAU / $20M-revenue thresholds + auto-termination. | LICENSE.md (verbatim) |
| 8 | Nougat weight license CC-BY-NC | **Confirmed exact.** "Nougat model weights are licensed under CC-BY-NC." | README + LICENSE |
| 9 | Nougat maintenance risk (stale) | **Confirmed.** Last code push 2025-02-21 (~1.5y stale); not archived; 143 open issues. | GitHub API `pushed_at` |
| 10 | UReader ≠ MinerU (no conflation) | **Confirmed separate.** Owner `LukeForeverYoung` ≠ `opendatalab`; different repos. | GitHub API |
| 11 | UReader research-only | **Confirmed.** mPLUG-Owl research env; A100/V100; no service/Docker/release. | README |
| 12 | UReader weight license | **Could NOT verify — UNDOCUMENTED.** Repo silent; HF card = "No model card". | README + HF page |
| 13 | Mathpix remote API only | **Mostly confirmed.** Standard Convert API is remote SaaS; a separate "On-prem PDF Cloud" enterprise product exists. | Pricing page |
| 14 | Mathpix data-residency / no-training | **Retention + no-training confirmed; processing REGION not stated.** | Pricing FAQ |
| 15 | pdf-inspector Cargo.toml 0.1.7 vs README 0.2.6 | **Resolved — both true, different artifacts.** crates.io/Rust = 0.1.7; PyPI/Python = 0.2.6; they version independently. | PyPI + crates.io |
| 16 | pdf-inspector selective-OCR signals in API | **Confirmed all three:** `pages_needing_ocr`, `ocr_reasons_by_page`, `has_encoding_issues`. | `docs/python.md` type stubs |

---

## Per-tool verified fact sheets

Each row cites the exact file/page read.

### 1. GROBID — `grobidOrg/grobid`

**Sources read:**
- GitHub REST API `https://api.github.com/repos/grobidOrg/grobid` (repo metadata)
- README via GitHub API (canonical root README is named `Readme.md`; raw `README.md` 404s)
- `https://grobid.readthedocs.io/en/latest/Grobid-service/` — **HTTP 429, could not load** (persistent rate-limit); GROBID-service-specific port/endpoint/release details NOT directly cited

| Dimension | Verified fact | Source |
|---|---|---|
| Canonical repo / org | `grobidOrg/grobid`. User's `krobidOrg` was a typo. `kermitt2` still owns auxiliary repos (biblio-glutton, grobid-client-{python,java,node}, delft, pdfalto, article-dataset-builder). | GitHub API `full_name` |
| Stars / forks / activity | 5055★, 565 forks, 4170 commits, `pushed_at` 2026-08-09, NOT archived. **Actively maintained.** | GitHub API |
| Deployment model | Java web service. README: "comprehensive web service API, Docker images, batch processing, a JAVA API". Build requires OpenJDK 21; optional Python 3.10–3.11 + JEP for Deep Learning. Demo servers on HuggingFace Spaces (CPU-only, CRF-only variant). | README |
| Default port | README's Play-With-Docker stack "opens a browser tab on **port 8080**" (PWD demo context). The canonical GROBID service default is **8070** per the readthedocs Grobid-service page, which **rate-limited (429)** and could not be cited directly — treat 8070 as well-known but not re-derived here. | README (8080); readthedocs (429, not cited) |
| REST API shape | Documented "web service API" exists with endpoints for header / fulltext / references / citation-context extraction (per GROBID service docs). **Specific endpoint list NOT directly cited** (readthedocs 429). Do not treat the endpoint names as verified here. | README (existence); readthedocs (not loaded) |
| GPU required? | **Optional.** README: "[Optional] NVIDIA GPU with CUDA support for faster Deep Learning models." Default config: "by default the Deep Learning models are not used, only CRF are selected ... to accommodate 'out of the box' hardware." | README |
| License (code) | **Apache-2.0** (SPDX). Docs = CC-0; annotated data = CC-BY. | GitHub API `license` + README |
| Capabilities (verbatim) | "Header extraction and parsing"; "References extraction and parsing" (~0.87 F1 PubMed / ~0.90 bioRxiv w/ DL); "Citation contexts recognition and resolution" (0.76–0.91 F1); "Full text extraction and structuring"; **"PDF coordinates for extracted information, allowing to create augmented interactive PDF based on bounding boxes of the identified structures"** → coordinates/bounding boxes CONFIRMED. Also names, affiliations, dates, consolidation via biblio-glutton/CrossRef (>0.95 F1 DOI/PMID), patent references, funders, copyright/license ID. 68 final labels. | README |
| Production signal | Named production users: ResearchGate, Semantic Scholar, HAL, scite.ai, Academia.edu, Internet Archive Scholar, INIST-CNRS, CERN (Invenio). | README |
| Release cadence | **Not directly verified** (readthedocs 429; GitHub API releases not pulled). Activity is high (`pushed_at` 2026-08-09, 4170 commits). Re-derive from GitHub Releases for cadence. | — |

**Open GROBID gap:** RESOLVED — see the `## GROBID deployment-fact addendum` section at the end of this file (port 8070/8071, 21 REST endpoints, release cadence, Docker image `grobid/grobid`, synchronous engine pool).

---

### 2. Marker — `datalab-to/marker` (weight upstream: `datalab-to/surya`)

**Sources read:**
- GitHub REST API `datalab-to/marker` and `datalab-to/surya`
- Both repos' READMEs (raw)

| Dimension | Verified fact | Source |
|---|---|---|
| Stars / activity | Marker 38594★, `pushed_at` 2026-08-07. Surya 21234★, `pushed_at` 2026-07-23. Both NOT archived. **Actively maintained.** | GitHub API |
| Deployment model | Local Python, `pip install marker-pdf` (or `marker-pdf[full]` for non-PDF). Surya inference server auto-spawns: **vllm** (NVIDIA GPU; needs Docker + NVIDIA Container Toolkit) or **llama.cpp/llama-server** (CPU / Apple Silicon). | README |
| Bundled HTTP API | Yes, minimal: `marker_server --port 8001` (FastAPI). README: "not a very robust API, only intended for small-scale use". Exposes only `page_range` / `mode` / `force_ocr` / `paginate_output` / `output_format` (does NOT expose `--use_llm` / `--disable_ocr`). No bundled Docker image (Modal example exists). | README |
| GPU required? | **Optional.** Modes: `--mode balanced` (default on GPU; Surya VLM) vs `--mode fast` (default on CPU/MPS; lightweight rf-detr layout + pdftext, VLM only for equations + surgical repair). `--disable_ocr` = pure CPU text-layer. | README |
| **CPU/GPU parity** | **NOT parity — different pipelines, different quality.** Fast-mode math "lower by design: reads equations from the PDF text layer rather than VLM-OCRing them, so LaTeX-level math tests mostly miss"; no-OCR mode "math scores zero". Benchmark overall: balanced 76.0 / fast 66.6 / no-OCR(CPU) 43.6. | README |
| **Selective OCR (brief's claim)** | **CONFIRMED, page + block granularity.** README: `--page_range` to select pages; balanced "re-OCRs the whole page whenever any of its embedded text is bad"; fast "surgical block-level repair of individual garbled/empty blocks, and a single full-page pass only for pages that are scanned or mostly bad"; `--force_ocr`; "Decide per page whether the embedded text is usable; garbled or scanned pages are OCR'd by the VLM." | README |
| Output | markdown / json / html / chunks; tables as HTML; equations as fenced `$$` LaTeX; images extracted; page+block structure with polygon bounding boxes (JSON `polygon`, 4-corner); TOC; multilingual OCR via Surya. | README |
| Quality-claim basis | **Reproducible, not marketing-only.** olmocr-bench (1403 PDFs), harness under `benchmarks/`, competitor runners incl. MinerU/docling. | README |
| License (code) | **Apache-2.0** (both Marker and Surya). | GitHub API `license` |
| **License (weights)** | **Modified AI-Pubs OpenRAIL-M.** Both READMEs (identical text): "The model weights use a **modified AI Pubs Open Rail-M license** (free for research, personal use, and startups under $5M funding/revenue). For broader commercial licensing of the model weights, visit our pricing page." Badge labeled "OpenRAIL-M". Exact full text travels with the weights on HuggingFace. | Marker + Surya README |
| Commercial use (weights) | **CONDITIONAL.** Free below $5M funding/revenue; paid license required above. NOT free-for-all. | README |

---

### 3. MinerU — `opendatalab/MinerU`

**Sources read:**
- GitHub REST API `opendatalab/MinerU`
- README (raw) — backend table
- `LICENSE.md` (raw, verbatim quotes below)

| Dimension | Verified fact | Source |
|---|---|---|
| Stars / activity | 77207★, `pushed_at` 2026-08-08, NOT archived, latest release 3.4 (2026-06-18). **Actively maintained.** GitHub API `license.spdx_id` = `NOASSERTION` (custom). | GitHub API |
| Deployment model | Multi-backend. `pipeline` (CPU-capable) / `hybrid-engine` / `vlm-engine` (GPU) / `*-http-client` (routes to OpenAI-compatible remote server via vLLM/SGLang/LMDeploy). Docker (Linux/WSL2 only), built-in FastAPI, Gradio WebUI, CLI, `mineru-router` for multi-GPU routing. | README backend table |
| Service / async | FastAPI built-in. Async: **YES** — 3.0.0 added async `POST /tasks` (`mineru-api`); sync `POST /file_parse` retained. | README / changelog |
| GPU required? | Optional (pipeline backend is pure-CPU ✅; hybrid/vlm require GPU). GPU accel: "Volta and later architecture GPUs or Apple Silicon". OS Linux/Win/macOS; Python 3.10–3.13. | README |
| **CPU/GPU parity (brief's claim)** | **RESOLVED — different pipelines, no parity.** `pipeline` (pure CPU ✅, min VRAM 4GB) accuracy **86.47** (OmniDocBench v1.6) vs `hybrid`/`vlm-engine` (pure CPU ❌, min VRAM 8GB) accuracy **~95.3** (95.39 high / 95.26 medium / 95.30 vlm). `*-http-client` pure CPU ✅, GPU not required. | README backend table |
| Capabilities | Formulas→LaTeX; tables→HTML; images + image descriptions; OCR 109 languages w/ auto-detect of scanned/garbled PDFs; multi-format input PDF/image/DOCX/PPTX/XLSX; removes headers/footers; reading order; `effort` (medium/high) for hybrid. | README |
| License (code) | **Custom "MinerU Open Source License"** — `LICENSE.md`: "MinerU is licensed under Apache License 2.0 and is subject to the additional terms below." | LICENSE.md |
| Commercial terms (verbatim) | §1: "MinerU may be used for commercial purposes without a separate commercial license" BUT if "monthly active users (MAU) exceed 100 million; or total monthly revenue exceeds USD 20 million" → separate commercial license required. §2 Attribution: "If you provide online services to third parties based on MinerU, you must clearly and prominently indicate ... that MinerU is used." §3 Termination: "this License and all rights granted under this License will terminate automatically" if commercial license not obtained OR attribution not met. | LICENSE.md |
| License history | Changelog: 3.1.0 (2026-04-18) moved AGPLv3 → this custom license; 3.0.0 removed 2 AGPLv3 models (doclayoutyolo, mfd_yolov8) + 1 CC-BY-NC-SA-4.0 model (layoutreader). | Changelog |
| Commercial use (code) | **Permitted up to thresholds** (100M MAU or $20M revenue), with **mandatory attribution**, else auto-terminates. | LICENSE.md |

---

### 4. Mathpix — `docs.mathpix.com` / `mathpix.com/pricing/api`

**Sources read:**
- `https://mathpix.com/pricing/api` (pricing page + FAQ; **time-sensitive, fetched 2026-08-10**)
- Docs site `docs.mathpix.com` referenced as API reference (not deeply traversed)

| Dimension | Verified fact | Source |
|---|---|---|
| Deployment model | **Remote SaaS API (Convert API).** A separate **"On-prem PDF Cloud"** enterprise product exists (nav link `/on-prem-pdf-cloud`) and a **Secure Conversion Service (SCS)** for batch — these are distinct offerings, not the standard Convert API. No self-host of the standard Convert API. | Pricing page |
| **Pricing (time-sensitive)** | One-time setup **$19.99**. **PDF `v3/pdf`: $0.005/page (0–1M), $0.0035/page (1M+).** Image (`v3/text`,`v3/latex`,`v3/batch`,`v3/strokes` no live updates): $0.002/image (0–1M), $0.0015 (1M+). Strokes w/ live updates: 0–1K free, 1K–100K $0.01/session, 100K–1M $0.008, 1M+ $0.005. Images with >12 text rows billed at PDF per-page rate. Billed monthly (1st). $29 testing credit. Enterprise = custom. | Pricing page |
| Capabilities | STEM text, equations (LaTeX), tables (CSV/TSV/LaTeX/Markdown), chemistry (SMILES, ChemDraw), PDF→LaTeX/Markdown/DOCX/HTML. 150+ parameters. | Pricing + product nav |
| **Data handling (brief's claim)** | **Retention configurable + no-training commitment present; processing REGION not stated.** FAQ: images "temporarily stored for quality assurance"; "Default retention is 30 days. You can opt out, and images are deleted within 24 hours." "We do not use your end-users' images for any purpose other than providing the requested service." Data Retention Opt-Out: "No Data Retention" (deleted within 24h, never saved to disk) / "Immediate Deletion" via support. SOC 2 badge. | Pricing FAQ |
| Data residency (region) | **Not documented on the pricing page.** Privacy Policy (`/privacy`) + SCS would need checking for a processing-region commitment. | Pricing page (silent) |
| License (API use) | **Proprietary commercial SaaS**, governed by Mathpix Terms (`/terms`). Not OSS-licensed. | Pricing page / nav |

---

### 5. Nougat — `facebookresearch/nougat`

**Sources read:**
- GitHub REST API `facebookresearch/nougat`
- README (raw) + `LICENSE` (raw)

| Dimension | Verified fact | Source |
|---|---|---|
| Stars / activity | 10056★, `pushed_at` **2025-02-21** (~1.5y before env date), NOT archived, 143 open issues. `updated_at` 2026-08-09 is metadata-only. | GitHub API |
| **Maintenance (brief's claim)** | **STALE / DORMANT — risk confirmed.** No code activity since 2025-02-21; not archived; 143 open issues. | GitHub API `pushed_at` |
| Deployment model | Local Python CLI `nougat path/to/file.pdf -o out` (directory/batch input supported). HTTP API via `nougat_api` (extra deps `nougat-ocr[api]`): POST to `http://127.0.0.1:8503/predict/` with `start`/`stop` page params. | README |
| Default port | **8503** (`nougat_api` HTTP server). | README |
| GPU required? | Not strictly required. README: `--full-precision` "Use float32 instead of bfloat16. Can speed up CPU conversion." FAQ: "false positives in the failure detection, when computing on CPU or older GPUs" — so **CPU works but with a known limitation** (failure-detection false positives; mitigation `--no-skipping`). | README |
| Batch / multi-page | **Yes.** `--batchsize`; `--pages '1-4,7'` (single PDF); directory/multi-file input. | README |
| Capabilities | "academic document PDF parser that understands LaTeX math and tables". Output `.mmd` (Mathpix-Markdown-compatible). Best with English arXiv/PMC-style papers; **"Chinese, Russian, Japanese etc. will not work."** Models `0.1.0-small` (default), `0.1.0-base`. | README |
| License (code) | **MIT** (SPDX). `LICENSE`: "Copyright (c) Meta Platforms, Inc. and affiliates." | LICENSE + GitHub API |
| **License (weights)** | **CC-BY-NC** (exact). README "License" section: "Nougat codebase is licensed under MIT. **Nougat model weights are licensed under CC-BY-NC.**" | README |
| Commercial use (weights) | **NOT permitted (NonCommercial).** Attribution required (BY). | README (CC-BY-NC) |

---

### 6. UReader — `LukeForeverYoung/UReader`

**Sources read:**
- GitHub REST API `LukeForeverYoung/UReader`
- README (raw)
- HuggingFace `https://huggingface.co/Mizukiluke/ureader-v1` (model page)

| Dimension | Verified fact | Source |
|---|---|---|
| **Separate from MinerU (brief's claim)** | **CONFIRMED separate.** Owner `LukeForeverYoung`; MinerU owner is `opendatalab`. Different repos, different owners, different projects (no conflation). | GitHub API `full_name` + `owner` |
| Stars / activity | 142★, 13 forks, `pushed_at` **2024-02-13** (~2.5y stale), NOT archived, no description, no homepage, no releases, 10 open issues, 3 subscribers. | GitHub API |
| **Research-only (brief's claim)** | **CONFIRMED research-only.** README is entirely training/inference/eval scripts. Env follows mPLUG-Owl: PyTorch 1.13.1 + CUDA 11.7 + transformers 4.29.1. Training requires **A100 80G** or **V100 32G**. Inference via `pipeline/interface.py` + `pipeline/evaluation.py`; offline demo `python -m app`. **No HTTP service, no Docker, no packaged CLI, no releases.** | README |
| GPU required? | **Yes** (CUDA 11.7; A100/V100 class). No CPU path documented. | README |
| License (code) | **Apache-2.0** (GitHub API `license`). | GitHub API |
| **License (weights)** | **UNDOCUMENTED — could not verify.** Repo README does not state a weight license. HF model page `Mizukiluke/ureader-v1` shows **"No model card"** (5 likes, 8 downloads/month, `mplug-owl` tag). Checkpoint derived from mPLUG-Owl (`MAGAer13/mplug-owl-llama-7b`), whose own weight terms may apply downstream. **Legal review MUST read the HF card + mPLUG-Owl terms before any use beyond research.** | README + HF page |
| Commercial use (weights) | **Indeterminate** (no license stated). Cannot be assumed permitted. | HF "No model card" |

---

### 7. pdf-inspector — `firecrawl/pdf-inspector`

**Sources read:**
- `https://pypi.org/project/pdf-inspector/` (HTML; version + release history)
- `https://crates.io/api/v1/crates/pdf-inspector` (JSON; versions)
- README (raw) and `docs/python.md` (raw)

| Dimension | Verified fact | Source |
|---|---|---|
| **Version-per-registry (brief's flag)** | **RESOLVED — both true, different artifacts.** **PyPI latest = 0.2.6** (released 2026-07-31). PyPI history: 0.1.0/0.1.1 (Mar 12), 0.2.0 (Jun 2), 0.2.1–0.2.5 (Jul 15), 0.2.6 (Jul 31). **crates.io latest = 0.1.7** (2026-07-31), 8 versions 0.1.0→0.1.7, **no 0.2.x exists on crates.io**. The Python binding and the Rust crate **version independently** (0.2.x vs 0.1.x). The README benchmark cites "pdf-inspector 0.2.6" = the **Python** version; the brief's "Cargo.toml 0.1.7" = the **Rust** crate. | PyPI HTML + crates.io JSON |
| License | **MIT** (badge + LICENSE; PyPI classifier; crates.io `license: MIT`). | README + PyPI + crates.io |
| Deployment model | Pure-Rust library (no ML, no OCR — text-layer only). Bindings: Python (PyO3), Node.js (napi), browser WASM. CLI bins `pdf2md` / `detect-pdf` / `dump_ops`. Python: `pip install pdf-inspector` (prebuilt wheels CPython ≥3.8 Linux/macOS/Win). Rust: `cargo add pdf-inspector`. | README + PyPI |
| In-process vs sidecar | **In-process** (library; Python/Node/WASM bindings). No server. | README |
| **Selective-OCR routing signals (brief's claim)** | **ALL THREE CONFIRMED as public API** in `docs/python.md` type stubs: `PdfResult.pages_needing_ocr: list[int]`; `PdfResult.ocr_reasons_by_page: list[PageOcrReasons]` (`PageOcrReasons.page: int`, `reasons: list[str]`); `PdfResult.has_encoding_issues: bool` ("broken font encodings — consider OCR fallback"). Also `RegionText.ocr_reason: str | None` and `PageMarkdown.needs_ocr`/`ocr_reason`. README: `pages_needing_ocr` "enabling per-page OCR routing instead of all-or-nothing"; `ScanStrategy` incl. `Pages(vec)` for caller-specified pages. | `docs/python.md` + README |
| Classification output | `pdf_type` ∈ `text_based`/`scanned`/`image_based`/`mixed` + `confidence` 0.0–1.0 + per-page OCR routing. | `docs/python.md` |
| Quality-claim basis | **Reproducible** (opendataloader-bench, 200 PDFs; harness in repo; results branch). Note the README's headline benchmark numbers and the PyPI benchmark table **disagree** (README overall 0.875/speed 0.470s vs PyPI overall 0.875/speed 2.8s) — same corpus, different speed figures; treat as **marketing claim, reconcilable but not identical**. | README + PyPI page |

---

## Consolidated license matrix (legal-review first pass)

| Tool | Code license (SPDX) | Model weights license | Commercial use OK? | Attribution required? | Notes |
|---|---|---|---|---|---|
| **GROBID** | Apache-2.0 | n/a (CRF/DL models are code-bundled; DL via DeLFT) | **Yes** | Yes (Apache-2.0 notice) | Data CC-BY; docs CC-0. Cleanest license profile. |
| **Marker** | Apache-2.0 | **Modified AI-Pubs OpenRAIL-M** | **Conditional** — free only <$5M funding/revenue; paid above | Yes (OpenRAIL use-based restrictions) | Weights shared with Surya. Exact full text on HuggingFace. |
| **MinerU** | Custom "MinerU Open Source License" (Apache-2.0 + extra terms) | bundled (license moved AGPLv3→custom in 3.1.0; removed AGPLv3/CC-BY-NC-SA models in 3.0.0) | **Conditional** — OK under 100M MAU & <$20M revenue | **Yes, mandatory + prominent** (else auto-terminate) | Auto-termination clause is unusual; flag in legal review. |
| **Mathpix** | Proprietary SaaS (no OSS license) | n/a (service-side) | n/a (paid API) | n/a (ToS-governed) | Region not stated; retention configurable; no-training stated. |
| **Nougat** | MIT | **CC-BY-NC** | **NO (weights NonCommercial)** | Yes (CC-BY) | Code is permissive; **weights block commercial use**. |
| **UReader** | Apache-2.0 | **UNDOCUMENTED** ("No model card") | **Indeterminate** | Unknown | Derived from mPLUG-Owl; must read HF card + upstream terms. **Hard legal gap.** |
| **pdf-inspector** | MIT | n/a (no ML models) | **Yes** | Yes (MIT notice) | Pure Rust, no weights — no weight-license concern. |

**Legal-review headline:** GROBID and pdf-inspector are clean (Apache/MIT, no weight entanglement). Marker, MinerU, Nougat, and UReader all carry weight or extra-term restrictions that gate commercial use. UReader is the only one whose weight license is **entirely undocumented** — it is the largest legal unknown.

---

## Consolidated deployment matrix

| Tool | Form | CPU parity vs GPU | Default port | Async support | Self-host |
|---|---|---|---|---|---|
| **GROBID** | Sidecar (Java web service) | CPU = CRF default; GPU adds DL accuracy (same pipeline family) | 8080 (PWD demo, per README); 8070 canonical (not directly cited) | Batch processing API (documented) | Yes (Docker) |
| **Marker** | Local Python lib + minimal HTTP server | **No parity** — CPU `fast` is a reduced pipeline (math lower by design) | `marker_server` 8001 (minimal) | No (small-scale API only) | Yes (local) |
| **MinerU** | Local lib + sidecar (FastAPI/Docker) + remote-client | **No parity — different backends**: CPU `pipeline` 86.47 vs GPU `hybrid`/`vlm` ~95.3 | (FastAPI; port configurable) | **Yes** (`POST /tasks` async) | Yes (Docker Linux/WSL2) |
| **Mathpix** | **SaaS only** (standard Convert API) | n/a (server-side GPU) | n/a | Batch via SCS | No (On-prem PDF Cloud is a separate enterprise product) |
| **Nougat** | Local Python CLI + HTTP server | CPU works but failure-detection false positives (use `--no-skipping`) | `nougat_api` 8503 | No | Yes (local) |
| **UReader** | Local research scripts | CPU not documented (GPU required) | (offline demo `python -m app`) | No | Yes (local; research env) |
| **pdf-inspector** | **In-process** lib (Python/Node/WASM bindings) | n/a (CPU-only, no ML) | n/a (no server) | n/a | Yes (in-process) |

---

## Cross-cutting verification

### Commercial-use posture (every "weights" license)
- **GROBID**: clean (Apache-2.0; no weight restriction).
- **Marker/Surya**: conditional — OpenRAIL-M, free only under $5M threshold.
- **MinerU**: conditional — revenue/MAU thresholds + mandatory attribution + auto-termination.
- **Nougat**: **blocked** — CC-BY-NC weights (non-commercial).
- **UReader**: **indeterminate** — undocumented weights.
- **pdf-inspector**: clean (MIT, no weights).

### CPU/GPU parity (every "GPU-optional" claim)
- **GROBID**: same pipeline family; GPU adds DL accuracy on top of CRF default. Effectively parity of *features*, accuracy differs.
- **Marker**: **reduced pipeline on CPU** (`fast`/`no-ocr` modes; math explicitly lower).
- **MinerU**: **different pipelines** (`pipeline` 86.47 vs `hybrid`/`vlm` ~95.3) — not parity.
- **Nougat**: same model, but CPU has a documented failure-detection limitation.
- **UReader**: CPU not supported.
- **pdf-inspector**: CPU-only by design (no GPU path).

### Marketing-claim flags (quality/superiority claims with reproducible evidence?)
- **GROBID**: F1 figures cited (PubMed/bioRxiv) — reproducible datasets named.
- **Marker**: olmocr-bench harness + competitor runners in repo — **reproducible**.
- **MinerU**: OmniDocBench v1.6 accuracy figures per backend — reproducible benchmark named.
- **Mathpix**: "unmatched accuracy" (pricing tagline) — **marketing claim, no public benchmark** in what was read.
- **Nougat**: paper-arxiv badge (arXiv 2308.13418) — peer-reviewed basis.
- **UReader**: arXiv 2310.05126 — peer-reviewed basis.
- **pdf-inspector**: opendataloader-bench harness in repo — reproducible; **but** README vs PyPI benchmark tables disagree on speed (0.470s vs 2.8s) — flag as inconsistent marketing numbers.

---

## Findings

- **(finding)**: source=GROBID README, confidence=high, type=fact — GROBID provides PDF coordinates/bounding boxes; GPU optional (default CRF/CPU); code Apache-2.0.
- **(finding)**: source=Marker+Surya README, confidence=high, type=fact — Marker/Surya weights = modified AI-Pubs OpenRAIL-M; commercial free only <$5M; selective OCR documented at page+block granularity.
- **(finding)**: source=MinerU README backend table, confidence=high, type=fact — CPU `pipeline` and GPU `hybrid`/`vlm` are different backends with different accuracy (86.47 vs ~95.3); not feature parity.
- **(finding)**: source=MinerU LICENSE.md, confidence=high, type=fact — MinerU license is Apache-2.0 + attribution + 100M-MAU/$20M-revenue thresholds + auto-termination.
- **(finding)**: source=Nougat README+LICENSE, confidence=high, type=fact — Nougat code MIT, weights CC-BY-NC (non-commercial); stale since 2025-02-21.
- **(finding)**: source=UReader GitHub API+README+HF page, confidence=high, type=fact — UReader is a separate research-only project (≠ MinerU); weight license undocumented ("No model card").
- **(finding)**: source=Mathpix pricing page, confidence=high, type=fact — Mathpix PDF $0.005/page (0–1M)/$0.0035 (1M+); remote SaaS; 30-day default retention, opt-out 24h, no-training stated; region not stated. Time-sensitive.
- **(finding)**: source=PyPI + crates.io, confidence=high, type=fact — pdf-inspector PyPI=0.2.6, crates.io=0.1.7; independent versioning; `pages_needing_ocr`/`ocr_reasons_by_page`/`has_encoding_issues` all confirmed in `docs/python.md`.
- **(inference)**: confidence=medium, type=inference — GROBID canonical default port is most likely 8070 (readthedocs not loadable); the 8080 in README is the PWD-demo mapping. Treat as unverified-here.

## Contradictions

<!-- Explicit contradictions between tools' marketing and their own docs/licenses, or "None detected" besides the items below. -->
- **pdf-inspector internal**: README benchmark table (speed **0.470s** for 200 docs, "Refreshed July 31, 2026") vs PyPI benchmark table (speed **2.8s**, "Refreshed July 16, 2026"). Same "Overall 0.875" but materially different speed figures and refresh dates. Reconcile before citing a speed number.
- **Marker badge vs prose (terminology only, not a real contradiction)**: badge "OpenRAIL-M" vs prose "modified AI Pubs Open Rail-M" — same license, two labels.
- **MinerU "open source" framing vs terms**: README/API presents as open source, but the license carries commercial thresholds, mandatory attribution, and auto-termination — not stock open-source by OSI criteria. Flag for legal review (not a factual contradiction, a perception gap).
- **Otherwise: None detected** across tools' own capability/deployment claims versus their docs/licenses.

## Sources

Every claim above cites one of these, read on 2026-08-10:

- GROBID: `https://api.github.com/repos/grobidOrg/grobid`; README via GitHub API (root file `Readme.md`). `https://grobid.readthedocs.io/en/latest/Grobid-service/` (HTTP 429 — NOT loaded).
- Marker: `https://api.github.com/repos/datalab-to/marker`; `https://raw.githubusercontent.com/datalab-to/marker/master/README.md`.
- Surya: `https://api.github.com/repos/datalab-to/surya`; Surya README (raw).
- MinerU: `https://api.github.com/repos/opendatalab/MinerU`; `https://raw.githubusercontent.com/opendatalab/MinerU/master/README.md`; `https://raw.githubusercontent.com/opendatalab/MinerU/master/LICENSE.md`.
- Mathpix: `https://mathpix.com/pricing/api` (time-sensitive).
- Nougat: `https://api.github.com/repos/facebookresearch/nougat`; `https://raw.githubusercontent.com/facebookresearch/nougat/main/README.md`; `https://raw.githubusercontent.com/facebookresearch/nougat/main/LICENSE`.
- UReader: `https://api.github.com/repos/LukeForeverYoung/UReader`; `https://raw.githubusercontent.com/LukeForeverYoung/UReader/main/README.md`; `https://huggingface.co/Mizukiluke/ureader-v1`.
- pdf-inspector: `https://pypi.org/project/pdf-inspector/`; `https://crates.io/api/v1/crates/pdf-inspector`; `https://raw.githubusercontent.com/firecrawl/pdf-inspector/main/README.md`; `https://raw.githubusercontent.com/firecrawl/pdf-inspector/main/docs/python.md`.

---

## Recommended next step

This is a **source packet**, not a decision memo. With ground-truth facts now in
hand, the natural downstream step is a `debate`/`planner` pass that turns these
into architecture options for the pipeline (e.g. which extractor is the CPU
default, which is the GPU high-accuracy path, which is the SaaS fallback, and
how pdf-inspector routes between them). Open inputs that pass should carry
forward: Marker/MinerU/Nougat/UReader all have weight/commercial restrictions
that constrain a production pipeline; UReader's undocumented weight license
makes it the weakest candidate for any non-research use. A follow-up fetch of
the GROBID readthedocs service page (when rate-limit clears) would close the
last minor gap (default port 8070, REST endpoint list, release cadence).

<!--
  PROVENANCE ADDENDUM: closes the GROBID deployment-fact gaps left by the
  2026-08-10 first pass (which could not cite port / endpoint list / release
  cadence because grobid.readthedocs.io returned HTTP 429 on every fetch).
  This pass bypassed readthedocs by reading the in-repo `doc/` Markdown
  (the readthedocs *source* — `doc/` contains `readthedocs.js`,
  `overrides/main.html`, `requirements.txt`, i.e. it is an MkDocs site built
  from these files), plus GitHub REST API + raw files and Docker Hub API.
  CLAIM CLASS: Class A (upstream primary) throughout. Every claim cites the
  exact URL/file read. Where a doc is silent, it is stated as "not documented".
-->

## GROBID deployment-fact addendum

**Date:** 2026-08-10. **Mode:** read-only source-gathering. **Purpose:** close
the port / REST-endpoint / release-cadence / Docker-image / healthcheck gaps so
the adopter can wire GROBID as a sidecar without guessing.

### 1. Canonical repo (proven by GitHub API)

**Canonical: `grobidOrg/grobid`.** The prior ambiguity is resolved by a
**repo transfer**, not a fork:

- `GET https://api.github.com/repos/kermitt2/grobid` returns the *same object*
  as `GET https://api.github.com/repos/grobidOrg/grobid` — identical `id`
  (`5797013`), `full_name: "grobidOrg/grobid"`, `fork: false`. GitHub
  transparently redirects the old owner path. There is **no live `kermitt2/grobid`
  repo** — that path is a permanent redirect, not a mirror.
- Transfer timestamp evidence: the `0.8.2` release notes (published 2025-05-11)
  still cite `github.com/kermitt2/grobid/compare/0.8.1...0.8.2`; the `0.9.0`
  release notes (published 2026-04-07) cite `github.com/grobidOrg/grobid/...`.
  The transfer happened between those two dates. `kermitt2` (Luca Foppiano,
  the GROBID author) remains the release publisher (`author.login: "lfoppiano"`
  on every release) and still owns the **auxiliary** repos
  (`kermitt2/biblio-glutton`, `kermitt2/grobid-client-{python,java,node}`,
  `kermitt2/delft`, `kermitt2/pdfalto`).

`grobidOrg/grobid` activity snapshot (fetched 2026-08-10):

| Field | Value |
|---|---|
| `full_name` | `grobidOrg/grobid` |
| `pushed_at` | `2026-08-09T17:41:00Z` (1 day before fetch) |
| `open_issues_count` | 298 |
| `forks_count` | 565 |
| `stargazers_count` | 5056 |
| `default_branch` | `master` |
| `license.spdx_id` | `Apache-2.0` |
| `archived` | `false` |
| `owner.type` | `Organization` (`grobidOrg`, id `252882185`) |
| `homepage` | `https://grobid.readthedocs.io` |

**Verdict: actively maintained, org-owned canonical repo.** Prior pass's
"confirmed `grobidOrg/grobid`" stands, now with the transfer mechanism
documented.

### 2. Default service port (file + line)

**Application port `8070`; admin port `8071`.** Confirmed verbatim in the
Dropwizard server block of both shipped config files:

- `grobid-home/config/grobid.yaml` (default CRF config), `server:` block:
  ```yaml
  server:
      type: custom
      applicationConnectors:
      - type: http
        port: 8070
      adminConnectors:
      - type: http
        port: 8071
  ```
- `grobid-home/config/grobid-full.yaml` (Deep-Learning config): identical
  `8070` / `8071`.

The prior pass's "well-known but uncited 8070" is now **cited from the primary
config**. The `8080` seen earlier is the **PWD-demo / host-mapping** port, not
the service default — `doc/Grobid-docker.md` explicitly frames 8080 as an
optional host remap: "the default version is running on port `8070`, however it
can be mapped on the more traditional port `8080` of your host with ...
`-p 8080:8070`". Source URLs:
- `https://raw.githubusercontent.com/grobidOrg/grobid/master/grobid-home/config/grobid.yaml`
- `https://raw.githubusercontent.com/grobidOrg/grobid/master/grobid-home/config/grobid-full.yaml`
- `https://raw.githubusercontent.com/grobidOrg/grobid/master/doc/Grobid-docker.md`

### 3. REST API endpoint table

All endpoints below are documented in `doc/Grobid-service.md` (83 KB, the
readthedocs source file). Base URL `http://host:8070`; admin/health/metrics on
`http://host:8071`. "Notable params" lists only params beyond the required
input; full param semantics in the cited doc.

**Service checks (no body)** — `doc/Grobid-service.md` → "Service checks":

| Method | Path | Purpose | Response |
|---|---|---|---|
| GET | `/api/version` | version + git revision (`<tag>-<N>-g<hash>`) | plain text |
| GET | `/api/isalive` | **liveness** probe | `true`/`false` plain text; HTTP 200 when alive, **503** when not initialized |
| GET | `/api/health` | **readiness** probe (init state, engine-pool metrics active/idle/max, config checks) | JSON; HTTP 200 ready, **503** otherwise |

> **Name correction:** the healthcheck path is `/api/isalive` (lowercase),
> **not** `/api/isAlive`. The CamelCase form does not appear in the primary
> doc.

**PDF → TEI/BibTeX** (input: `multipart/form-data`, field `input` = PDF file):

| Method | Path | Output | Notable params |
|---|---|---|---|
| POST, PUT | `/api/processHeaderDocument` | TEI XML (or BibTeX via `Accept: application/x-bibtex`) | `consolidateHeader` (0/1/2/3), `includeRawAffiliations`, `includeRawCopyrights`, `start`, `end`, `debugMode`, `models` |
| POST, PUT | `/api/processFulltextDocument` | TEI XML | `consolidateHeader`, **`consolidateCitations`** (0/1/2), `consolidateFunders`, `includeRawCitations`, `includeRawAffiliations`, `includeRawCopyrights`, `teiCoordinates`, **`segmentSentences`**, `generateIDs`, `start`, `end`, `flavor`, `debugMode`, `models` |
| POST, PUT | `/api/processReferences` | TEI XML (or BibTeX) | `consolidateCitations`, `includeRawCitations`, `debugMode`, `models` |
| (debug-ref'd) | `/api/processFulltextAssetDocument` | ZIP (TEI + extracted assets) | supports `debugMode` per the debug-mode paragraph; no dedicated `####` section in current doc |

**Raw text → TEI** (input: `application/x-www-form-urlencoded`):

| Method | Path | Input field | Output |
|---|---|---|---|
| POST, PUT | `/api/processDate` | `date` | TEI date fragment |
| POST, PUT | `/api/processHeaderNames` | `names` | TEI `<persName>` |
| POST, PUT | `/api/processCitationNames` | `names` | TEI `<persName>` |
| POST, PUT | `/api/processAffiliations` | `affiliations` | TEI `<affiliation>` |
| POST, PUT | `/api/processCitation` | `citations` (single ref) | TEI `<biblStruct>` (or BibTeX); params `consolidateCitations`, `includeRawCitations` |
| POST | `/api/processCitationList` | `citations` (list of refs) | TEI (or BibTeX); params `consolidateCitations`, `includeRawCitations` |

**PDF annotation** (input: `multipart/form-data`, `input` = PDF):

| Method | Path | Output | Notable params |
|---|---|---|---|
| POST | `/api/referenceAnnotations` | JSON (coords for ref callouts + bib. links) | `consolidateCitations`, `includeRawCitations`, `includeFiguresTables` |
| POST | `/api/annotatePDF` | augmented PDF (`application/pdf`; modifies input — may be deprecated) | `consolidateCitations` |

**Patent citations** (three input encodings + annotation):

| Method | Path | Input | Output |
|---|---|---|---|
| POST, PUT | `/api/processCitationPatentTXT` | UTF-8 text (`application/x-www-form-urlencoded`) | TEI |
| POST, PUT | `/api/processCitationPatentST36` | ST.36 XML file (`multipart/form-data`) | TEI |
| POST, PUT | `/api/processCitationPatentPDF` | patent PDF (`multipart/form-data`; needs text layer) | TEI |
| POST | `/api/citationPatentAnnotations` | patent PDF | JSON annotations w/ coords |

All four patent endpoints accept `consolidateCitations` (0/1/2); the three
`process*` ones also accept `includeRawCitations`.

**Training web API** (model lifecycle; not for document processing):

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/modelTraining` | launch training, returns token; **409** if same model already training |
| POST | `/api/trainingResult` | poll advancement / get evaluation by token |
| GET | `/api/allTraining` | list live training tokens (in-memory; cleared on restart) |
| DELETE | `/api/killTraining?token=` | interrupt a training |
| GET/POST | `/api/model` | download a model archive (`.zip`) |
| POST | `/api/createTraining` | generate training-data ZIP from a PDF (optional `flavor`) |

**`consolidateCitations` / `consolidateDoi` / `includeRawAffiliations` /
`segmentSentences` specifically:**
- `consolidateCitations` — present on `processFulltextDocument`,
  `processReferences`, `processCitation`, `processCitationList`, all patent
  `process*`, `referenceAnnotations`, `annotatePDF`. Values `0` (default,
  none) / `1` (consolidate + inject all metadata) / `2` (DOI only).
- **`consolidateDoi`** — **not a documented parameter name.** The "DOI only"
  behavior is selected via `consolidateCitations=2` (or `consolidateHeader=2`).
  `consolidateDoi` does not appear in `doc/Grobid-service.md`.
- `includeRawAffiliations` — present on `processHeaderDocument` and
  `processFulltextDocument` only.
- `segmentSentences` — present on `processFulltextDocument` only.
- Header consolidation adds a fourth value `3` (DOI-only path) on
  `consolidateHeader`.

**Output formats:** server always returns **TEI XML** as the source of truth.
**BibTeX** is available on the four bibliographic endpoints via
`Accept: application/x-bibtex`. **Markdown / JSON** are *client-side*
projections performed by `grobid-client-python` (lossy; CORD-19-inspired JSON),
NOT server endpoints.

Source: `https://raw.githubusercontent.com/grobidOrg/grobid/master/doc/Grobid-service.md`

### 4. Release cadence (last 7 releases, GitHub Releases API)

From `https://api.github.com/repos/grobidOrg/grobid/releases?per_page=8`:

| Tag | Published | Publisher | Gap | Notes |
|---|---|---|---|---|
| `0.9.1` | 2026-08-04 | `lfoppiano` | — | current latest; OTLP metrics, rootless Docker, pdfalto 0.6.2, security CVE fixes |
| `0.9.0` | 2026-04-07 | `lfoppiano` | ~4 mo | JDK 21, Gradle 9, TensorFlow 2.17, ARM64 Docker, `/api/health` endpoint added (#1373) |
| `0.8.2` | 2025-05-11 | `lfoppiano` | ~12 mo | model flavors mechanism; **last release whose notes still cite `kermitt2/grobid` URLs** |
| `0.8.1` | 2024-09-14 | `lfoppiano` | ~8 mo | DL patent models, paragraph coordinates, biblio-glutton 0.3 |
| `0.8.0` | 2023-11-26 | `kermitt2` | ~9 mo | Dropwizard 4.0, funder extraction; **last release published by `kermitt2` login** |
| `0.7.3` | 2023-05-13 | `kermitt2` | ~6 mo | JDK>11 support, Mac ARM, PWD docs |
| `0.7.2` | 2022-11-21 | `kermitt2` | — | baseline |

**Verdict: STEADY-ACTIVE.** Roughly one minor release every 6–12 months,
accelerating in the last year (0.9.0 → 0.9.1 in ~4 months). Latest release 6
days before fetch; `pushed_at` 1 day before fetch. Not stale. The publisher
identity shifted from `kermitt2` to `lfoppiano` between 0.8.0 and 0.8.1
(same person, GitHub login change), and the org transfer from `kermitt2`→
`grobidOrg` was completed between 0.8.2 and 0.9.0.

### 5. Docker image

**Two equivalent registries sharing the same tags** (cited in
`doc/Grobid-docker.md` and confirmed against Docker Hub API):

| Image | Role | Status |
|---|---|---|
| `grobid/grobid` | **canonical** (org-owned, matches the GitHub org) | Docker Hub `active`, 356,689 pulls, 18★, `last_updated` 2026-08-04 |
| `lfoppiano/grobid` | **mirror** (maintainer-owned, predates the org) | carries the same tags |

**Two tag variants** (both available from *both* registries per the in-repo
doc; Docker Hub's per-repo descriptions emphasize one each but the deploy doc
is authoritative):

| Tag suffix | Content | Size | GPU |
|---|---|---|---|
| `-full` (e.g. `0.9.1-full`) | CRF + Deep Learning, all DL model resources + embeddings | ~8 GB compressed / **13.8 GB** on Docker Hub for `0.9.1-full` | recommended (≥4 GB VRAM); falls back to CPU |
| `-crf` (e.g. `0.9.1-crf`) | CRF models only | ~500 MB | not used |

**Default exposed port: `8070`** (application); admin/metrics `8071`. Both
must be published for full operability (admin port carries the healthcheck and
Prometheus scrape).

**Documented `docker run` invocations** (verbatim from `doc/Grobid-docker.md`):

Full image, GPU:
```bash
docker run --rm --gpus all --init --ulimit core=0 -p 8070:8070 grobid/grobid:0.9.1-full
```
Full image, GPU, with admin/metrics port published (production shape):
```bash
docker run --rm --gpus all --init --ulimit core=0 -p 8070:8070 -p 8071:8071 grobid/grobid:0.9.1-full
```
CRF-only image, CPU:
```bash
docker run --rm --init --ulimit core=0 -p 8070:8070 grobid/grobid:0.9.1-crf
```
Host-remap to 8080 (the prior pass's "8080" was this, not the service default):
```bash
docker run --rm --init --ulimit core=0 -p 8080:8070 -p 8081:8071 grobid/grobid:0.9.1-crf
```

Config override via mounted `grobid.yaml` (absolute host path required):
```bash
docker run --rm --gpus all --init --ulimit core=0 -p 8070:8070 -p 8071:8071 \
  -v /abs/path/grobid.yaml:/opt/grobid/grobid-home/config/grobid.yaml:ro \
  grobid/grobid:0.9.1-full
```

> **Sidecar note for the adopter:** the `-p 8070:8070 -p 8071:8071` form is the
> correct sidecar wiring; omit the `8071` publish only if you do not intend to
> scrape metrics or run the readiness probe. The in-container paths are
> `/opt/grobid/grobid-home/...` and `/opt/grobid/logs/`.

Sources: `https://raw.githubusercontent.com/grobidOrg/grobid/master/doc/Grobid-docker.md`,
`https://hub.docker.com/v2/repositories/grobid/grobid/`,
`https://hub.docker.com/r/grobid/grobid`.

### 6. Async / batch / healthcheck

| Capability | Status | Evidence |
|---|---|---|
| **Healthcheck — liveness** | **Yes.** `GET /api/isalive` → `true`/`false`, HTTP 200 alive / 503 not-initialized. Explicitly "Suitable for use as a liveness probe in container orchestrators (Docker, Kubernetes)." | `doc/Grobid-service.md` → Service checks |
| **Healthcheck — readiness** | **Yes.** `GET /api/health` → JSON (init state + engine-pool active/idle/max + config checks), HTTP 200 ready / 503 otherwise. Added in 0.9.0 (#1373), made stricter in 0.9.1. | `doc/Grobid-service.md`; 0.9.0/0.9.1 release notes |
| **Version endpoint** | **Yes.** `GET /api/version` → version + `git describe` revision. | `doc/Grobid-service.md` |
| **Admin console + metrics** | **Yes.** Admin console at `:8071/`; Prometheus exposition at `GET :8071/metrics/prometheus`; OTLP push (Grafana Cloud etc.) configurable under `grobid.otlp`. | `doc/Monitoring.md`, `grobid-home/config/grobid.yaml` `otlp:` block |
| **Batch PDF processing (REST)** | **NOT documented.** No `/api/processBatch` endpoint exists. | `doc/Grobid-service.md` has no such path |
| **Batch PDF processing (CLI)** | **Yes, but deprecated.** CLI-only batch via `grobid-core-*-onejar.jar -exe processX` (`processHeader`, `processFullText`, `processReferences`, `processCitationPatent*`, `createTraining`, etc.). The doc explicitly says: "We do **not** recommend to use the batch mode. For the best performance ... use the service ... and not the batch mode." | `doc/Grobid-batch.md` |
| **Recommended batch path** | Service + multithreaded client (Python/Java/Node/Go). Clients fan out requests across the engine pool. | `doc/Grobid-service.md` → Clients; `doc/Grobid-batch.md` deprecation banner |
| **Async / job-queue mode** | **NOT documented.** The service is **synchronous + multithreaded**: a bounded pool of engines (default `grobid.concurrency: 10`) serves requests; when exhausted, requests get HTTP **503** (with `grobid.poolMaxWait: 1` s grace). There is no submitted-job / poll-for-result API for document processing. (The only async-style endpoints are the **model-training** ones — `/api/modelTraining` returns a token, `/api/trainingResult` polls — but those train models, not documents.) | `doc/Grobid-service.md` → Parallel mode + `grobid.yaml` `concurrency`/`poolMaxWait` |

### 7. Sources read (every URL/file in this addendum)

GitHub REST API (`api.github.com`):
- `/repos/grobidOrg/grobid` (repo metadata — canonical proof)
- `/repos/kermitt2/grobid` (redirect proof — returns the `grobidOrg` object)
- `/repos/grobidOrg/grobid/releases?per_page=8` (cadence table)
- `/repos/grobidOrg/grobid/git/trees/master?recursive=0` (located `doc/`, `Dockerfile.*`)

Raw in-repo files (`raw.githubusercontent.com/grobidOrg/grobid/master/`):
- `grobid-home/config/grobid.yaml` (default port 8070/8071, Dropwizard block)
- `grobid-home/config/grobid-full.yaml` (DL config, same port)
- `doc/Grobid-service.md` (REST endpoint reference, 83 KB)
- `doc/Grobid-docker.md` (image names, run commands, port mapping)
- `doc/Grobid-batch.md` (deprecated CLI batch)
- `doc/Monitoring.md` (Prometheus/OTLP, admin-port metrics)
- 404'd (confirmed absent): `Dockerfile`, `docker-compose.yml` at repo root
  (actual Dockerfiles are `Dockerfile.crf`, `Dockerfile.delft`,
  `Dockerfile.evaluation`)

Docker Hub:
- `https://hub.docker.com/v2/repositories/grobid/grobid/` (API JSON: active,
  356K pulls, `last_updated` 2026-08-04, description "GROBID full image for
  using both Deep Learning models and CRF")
- `https://hub.docker.com/r/grobid/grobid` (tag `0.9.1-full`, 13.8 GB)

readthedocs (retry attempted):
- `https://grobid.readthedocs.io/en/latest/Grobid-service/` → **HTTP 429 again**
  (persistent rate-limit). **Fall-back used:** the in-repo `doc/Grobid-service.md`
  is the MkDocs *source* for that exact readthedocs page (`doc/` contains
  `readthedocs.js`, `overrides/main.html`, `requirements.txt`), so the
  primary-source content was obtained without readthedocs. This is stronger
  evidence than the rendered page would have been.

### 8. Contradictions / stale-guidance flags

- **`kermitt2/grobid` vs `grobidOrg/grobid`** — RESOLVED. Not a fork, a
  **transfer**; the old path redirects. The Docker Hub
  `grobid/grobid` repository *description* still says "See the GROBID GitHub
  repository → github.com/kermitt2/grobid" — that link works (redirects) but
  is **stale wording**; the canonical URL is `github.com/grobidOrg/grobid`.
  Flag for the adopter: any doc/tool hardcoding the `kermitt2/grobid` URL is
  relying on GitHub's redirect permanence.
- **Prior pass's "8070 inferred, not cited"** — RESOLVED. 8070 is now cited
  from `grobid.yaml`. The 8080 the prior pass saw in the README is the
  PWD-demo / host-remap port, not the service default.
- **`grobid/grobid` vs `lfoppiano/grobid` scope on Docker Hub** — MINOR.
  The in-repo `doc/Grobid-docker.md` says both registries carry both `-full`
  and `-crf` tags. The Docker Hub per-repo description for `grobid/grobid`
  frames `lfoppiano/grobid` as the CRF-only one. Treat the in-repo doc as
  authoritative (it shows explicit `docker pull grobid/grobid:${ver}-crf` and
  `docker pull lfoppiano/grobid:${ver}-crf` commands).
- **`/api/isAlive` (CamelCase)** — the prior task framing used CamelCase; the
  actual documented path is **lowercase `/api/isalive`**. Any client coded to
  the CamelCase form would 404.
- **`consolidateDoi`** — appears in the task framing but **not in the primary
  doc**; the DOI-only behavior is `consolidateCitations=2` (or
  `consolidateHeader=2`). Do not implement a `consolidateDoi` parameter.
- **`/api/texteller` / `/api/processAnnotation`** — the task framing
  hypothesized these; neither exists. The actual annotation endpoints are
  `/api/referenceAnnotations` and `/api/citationPatentAnnotations`; there is
  no "texteller" endpoint (no such feature in `doc/Grobid-service.md`).
- **Async/job-queue** — the task framing asked whether one exists; it does
  **not** for documents (synchronous + bounded pool only). Only model training
  has a token/poll async pattern.

## Findings
- **(finding)**: GROBID canonical repo is `grobidOrg/grobid`, transferred from `kermitt2` ~mid-2025; `kermitt2/grobid` is a redirect, not a fork. source=GitHub API (identical id 5797013, fork:false, release-note URL migration), confidence=high, type=fact
- **(finding)**: Default service port is 8070 (application) / 8071 (admin), Dropwizard connectors. source=`grobid-home/config/grobid.yaml` server block, confidence=high, type=fact
- **(finding)**: 21 documented REST endpoints + 3 service checks; healthcheck is lowercase `/api/isalive` (liveness) + `/api/health` (readiness); no `/api/isAlive`, no `/api/processBatch`, no `/api/texteller`. source=`doc/Grobid-service.md`, confidence=high, type=fact
- **(finding)**: Release cadence is steady-active (~6–12 mo between minors, accelerating; 0.9.1 published 2026-08-04, 6 days pre-fetch; pushed_at 1 day pre-fetch). source=GitHub Releases API, confidence=high, type=fact
- **(finding)**: Canonical Docker image is `grobid/grobid` (Docker Hub active, 356K pulls) with `lfoppiano/grobid` as mirror; two tag variants `-full` (DL+CRF, 13.8 GB) and `-crf` (CRF-only, ~500 MB); default exposed port 8070. source=`doc/Grobid-docker.md` + Docker Hub API, confidence=high, type=fact
- **(finding)**: No async/job-queue mode for document processing; service is synchronous with a bounded engine pool (default 10) returning HTTP 503 when saturated. source=`doc/Grobid-service.md` Parallel mode + `grobid.yaml` concurrency/poolMaxWait, confidence=high, type=fact
- **(finding)**: `consolidateDoi` is not a real parameter; DOI-only consolidation is `consolidateCitations=2` or `consolidateHeader=2`. source=`doc/Grobid-service.md`, confidence=high, type=fact

## Contradictions
<!-- Resolved contradictions listed in §8; flagged here per protocol. -->
- readthedocs HTTP 429 persisted on retry → bypassed by reading the in-repo `doc/` MkDocs source (equivalent-or-stronger primary evidence). No outstanding contradiction from this.
- Docker Hub `grobid/grobid` description still cites `github.com/kermitt2/grobid` (stale wording; works via redirect). Flagged for the adopter; canonical URL is `github.com/grobidOrg/grobid`.
- Docker Hub per-repo description scopes `lfoppiano/grobid` as "CRF-only"; in-repo doc says both registries carry both tags. In-repo doc treated as authoritative.

## Verification
| Claim | Verifying command/output | Verified |
|---|---|---|
| Canonical = grobidOrg | `GET api.github.com/repos/kermitt2/grobid` returns `full_name: grobidOrg/grobid`, `fork: false`, same `id` as the grobidOrg fetch | yes |
| Default port 8070/8071 | `grobid-home/config/grobid.yaml` `server.applicationConnectors[0].port: 8070`, `adminConnectors[0].port: 8071` | yes |
| 21 endpoints + 3 checks | grep `^#### /api/` in `doc/Grobid-service.md` → 20 `####` headers + `/api/createTraining` section + 3 service-check paths in prose | yes |
| Latest release 0.9.1 @ 2026-08-04 | GitHub Releases API `published_at: 2026-08-04T06:56:01Z`, tag `0.9.1` | yes |
| Docker image active | Docker Hub API `status: active`, `last_updated: 2026-08-04`, pull_count 356689 | yes |
| No async doc processing | absence of any submit/poll endpoint in `doc/Grobid-service.md`; only `/api/modelTraining`+`/api/trainingResult` (model training, not documents) | yes |

<!--
  END ADDENDUM. Intended to be appended verbatim to
  tmp/paper-pipeline-extractor-tool-facts-staging.md (and ultimately
  researches/sources/2026-08-10-paper-pipeline-extractor-tool-facts.md)
  under the "## GROBID deployment-fact addendum" heading above.
  ARTIFACT TYPE: source packet (Class A primary). NOT a decision memo.
-->
