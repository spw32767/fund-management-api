# Paper AI integration

The fund-management backend is the only browser-facing gateway for the two independent AI services:

- Paper Reader API (`PAPER_READER_API_URL`): PDF text extraction, Thai/English OCR, DOI detection inside the PDF, and Thai summaries.
- Paper Classification API (`PAPER_CLASSIFICATION_API_URL`): taxonomy-driven paper classification.

Both services use `PAPER_AI_API_KEY` as the `X-API-Key` value. They may run on different hosts and use different Ollama models.

## Internal application endpoints

- `POST /api/v1/paper-ai/extract` — multipart PDF (`file`)
- `POST /api/v1/paper-ai/summarize`
- `POST /api/v1/paper-ai/classify`
- `POST /api/v1/paper-ai/match` — send `{"doi":"10.x/...","benchmark_only":true}` for an exact DOI check against `scopus_benchmark_documents` only
- `GET /api/v1/paper-ai/categories`
- `GET /api/v1/paper-ai/jobs/:id` — read the authenticated user's audit record
- `POST /api/v1/admin/paper-ai/benchmark/:id/classify` — classify and persist one existing benchmark document

The classifier taxonomy is loaded from active `paper_categories` rows for every request. The browser never sends the authoritative taxonomy and never receives the service API key.

The active taxonomy contains exactly seven categories:

1. Theoretical Computer Science
2. AI Algorithms and Intelligent Systems
3. Applied AI, GeoAI and Agentic Applications
4. Networks, Security, and Distributed Systems
5. Quantum Information Science
6. Computer Engineering, IoT, and Embedded Systems
7. Educational Technology, Learning Sciences, and Digital Library Systems

Research papers receive one category. When the record is a preface or lacks enough evidence to classify, the category stays `NULL` and `classification_confidence` is `Preface` (shown as รอตรวจสอบ). Other confidence values are `High`, `Medium`, and `Low` (สูง, กลาง, ต่ำ). The classifier uses the abstract as primary evidence and the title and `authkeywords` as supporting evidence.
These confidence labels are reported by the local model; they are not calibrated probabilities and should be reviewed before bulk database updates.

In `scopus_benchmark_documents`, the `category` column is a nullable foreign key to `paper_categories.category_id`. The API exposes that value as `paper_category_id` so it remains explicit to clients. `publication_reward_details` uses the explicit `paper_category_id` column name for the same relationship.

On the publication reward form, the Reader API extracts PDF metadata and uses OCR when the PDF has no usable text layer. It rejects files without research-paper structure using a conservative document check. When explicitly present, the PDF can supply publication month, volume/issue, and page numbers; a validated DOI supplies the article URL. The form checks an extracted (or already entered) DOI only against `scopus_benchmark_documents` with `benchmark_only: true`. It links a benchmark document only when exactly one DOI match exists and displays whether the DOI was found. If the DOI is absent, missing from the benchmark, duplicated, or the check fails, the form still fills fields from the PDF and allows the applicant to continue to review. The original abstract and AI-generated Thai summary appear together. While a PDF is processing, the form disables its controls and shows a blocking loading dialog. This form does not call the Classification API or infer quartile or official database indexing from PDF text. DOI remains an editable field; there is no external DOI-reference lookup endpoint or button. Other callers may still use the general match endpoint's title and submission checks.

Calls currently complete in the initiating HTTP request. Each AI call is also recorded in `paper_ai_jobs`; extracted full text is deliberately omitted from the job log.

Apply migration `045_20260920_add_paper_ai_fields.sql` before enabling the form feature. On databases where 045 was already applied, run the separate `046_20260922_allow_paper_classification_preface.sql` migration to add the review status. These migrations do not change `eid` or insert non-Scopus papers into `scopus_benchmark_documents`.
