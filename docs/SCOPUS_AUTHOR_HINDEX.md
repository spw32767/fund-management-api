# Scopus Author h-index Subsystem — Handoff

ระบบดึงและแสดง **h-index รายอาจารย์** จาก Scopus พร้อมกราฟ Hirsch และการส่งออก CSV/รายงาน
เอกสารนี้ครอบคลุมทั้ง backend (`fund-management-api`) และ frontend (`frontend_project_fund`)
เพื่อให้ dev / AI agent รับงานต่อได้โดยไม่ต้องไล่อ่านโค้ดใหม่ทั้งหมด

> Branch: `feature/scopus-author-hindex` (ทั้ง 2 repo) — push แล้ว
> วันที่: 2026-08-16

---

## 1. ภาพรวม / จุดประสงค์

ต้องการ h-index ของอาจารย์แต่ละคนในระบบ โดยอิง `users.scopus_id` มี 2 แหล่ง/มุมมองที่ **ต่างกันสำคัญ**:

| แหล่ง | ได้อะไร | ใช้ที่ไหน |
|---|---|---|
| **Scopus Author Retrieval API** (`content/author/author_id/{id}?view=METRICS`) | ค่า h-index ทางการ **ค่าเดียว ณ ปัจจุบัน** (+ document/citation/coauthor counts) ไม่มีประวัติย้อนหลัง | เก็บ snapshot รายวันในตาราง `scopus_author_metrics` |
| **คำนวณเองจาก `scopus_documents`** (Hirsch h-graph) | h-index + กราฟ (เอกสารเรียงตาม citations vs เส้น y=x) กรองช่วงปีได้ | หน้า dashboard (กราฟ) + export |

**ทำไมต้องมีทั้งสอง:** Scopus ไม่คืนประวัติ h-index → ถ้าอยาก plot การเปลี่ยนแปลงตามเวลาต้องเก็บ snapshot เอง.
ส่วนกราฟแบบหน้า scopus.com (Hirsch) คำนวณจาก per-document citation ได้ทันที ไม่ต้องรอสะสม.
ปกติ **ค่าทั้งสองเท่ากัน** ถ้า ingest เอกสารครบและ citation สด (ต่างได้ถ้า ingest ไม่ครบ/citation เก่า).

---

## 2. Backend (`fund-management-api`)

### 2.1 Migrations (ต้องรันตามลำดับเลข)
- `migrations/039_20260816_create_scopus_author_metrics.sql`
  ตาราง **`scopus_author_metrics`** — snapshot append-only ของ Author API metrics
  key กันซ้ำ: `UNIQUE(scopus_author_id, snapshot_date)` → ค่าปัจจุบัน = แถวล่าสุด, กราฟตามเวลา = ทุกแถว
- `migrations/040_20260816_create_scopus_author_metrics_logs.sql`
  - **`scopus_author_metrics_runs`** — สรุปต่อการรัน batch (status/counts/api_calls/duration)
  - **`scopus_author_metric_requests`** — log ทุก HTTP request ไป Author API (ไว้ audit โควตา 5,000/สัปดาห์)

### 2.2 Models
- `models/scopus_author_metrics.go` → `ScopusAuthorMetric`
- `models/scopus_author_metrics_run.go` → `ScopusAuthorMetricsRun`, `ScopusAuthorMetricRequest`

### 2.3 Services
- **`services/scopus_author_metrics_service.go`** — `AuthorMetricsService`
  - `RunForAll(ctx, input)` — วน `users.scopus_id` → ยิง Author API ทีละคน → upsert snapshot รายวัน
    - บันทึก run + log ทุก request; กันรันซ้อนด้วย `GET_LOCK("scopus_author_metrics_job_lock")`
    - `input.TriggerSource` = `cli` / `admin_ui`
  - `RunForAuthor`, `GetActiveRun`
  - parse response: `author-retrieval-response[0].{coredata.*, h-index, coauthor-count}` (รองรับทั้ง array/object)
- **`services/scopus_author_hgraph_service.go`** — `AuthorHGraphService` (คำนวณจาก DB ล้วน ไม่ยิง API)
  - `GetGraph(ctx, scopusID, yearFrom, yearTo)` → `AuthorHIndexGraph{ h_index, document_count, citation_total, available_year_min/max, available_years[], points[] }`
    - `points` = เอกสารเรียง citations มาก→น้อย (rank, citations, title, year, eid) ใช้วาดกราฟ
    - `available_years` = ปี ค.ศ. ที่มีเอกสารจริง (distinct) — ให้ frontend ทำ dropdown ปี
  - `GetAllSummary(ctx)` → `[]AuthorSummaryRow` สำหรับ export ทุกคน (คำนวณ h-index ทุกคนใน 1 คิวรี + join ค่าทางการจาก `scopus_author_metrics`)
- **`services/named_lock.go`** — `releaseNamedLock()` helper (ดูข้อ 4.3)

### 2.4 Endpoints (กลุ่ม `/api/v1/admin` — role admin)
| Method / Path | Handler | ใช้ทำอะไร |
|---|---|---|
| `POST /scopus/author-metrics/refresh` | `AdminRefreshAuthorMetrics` | ดึง h-index ทุกคน (async) + กันรันซ้อน |
| `GET /scopus/author-metrics/runs` | `AdminListAuthorMetricsRuns` | ประวัติการรัน (paginated) |
| `GET /scopus/author-metrics/hgraph?scopus_id=&year_from=&year_to=` | `AdminGetScopusAuthorHIndexGraph` | ข้อมูลกราฟ Hirsch รายคน |
| `GET /scopus/author-metrics/summary` | `AdminGetAuthorHIndexSummary` | สรุป h-index ทุกคน (สำหรับ export CSV ทั้งหมด) |

controller: `controllers/admin_scopus_author_metrics_controller.go`
routes: `routes/routes.go` (บล็อก scopus admin)

### 2.5 CLI
- `cmd/scopus-author-metrics/` — รัน `RunForAll` จาก command line
  - flags: `--user-ids=1,2` (optional), `--limit=N` (optional)
  - `go run ./cmd/scopus-author-metrics`

---

## 3. Frontend (`frontend_project_fund`)

### 3.1 API layer — `app/lib/api.js` (`scopusConfigAPI`)
- `refreshAuthorMetrics({user_ids, limit})` · `listAuthorMetricsRuns(params)`
- `getAuthorHIndexGraph({scopus_id, year_from, year_to})` · `getAuthorHIndexSummary()`

### 3.2 หน้า Admin Scopus Import — `.../settings/announcement_config/AdminScopusImport.js`
เพิ่มการ์ด **"h-index อาจารย์ (Scopus)"**: ปุ่ม "ดึง h-index ทุกคน" + ตารางประวัติการรัน
(mirror pattern การ์ด conference/CiteScore เดิมในหน้าเดียวกัน)
route หน้า: `/research-fund-system/admin/academic-imports` (แท็บ Scopus)

### 3.3 การ์ดกราฟ h-index — `.../research/AdminScopusAuthorHIndex.js` (component ใหม่)
mount ท้ายหน้า **`/research-fund-system/admin/research-dashboard`** (`AdminScopusResearchDashboard.js`)
- dropdown เลือกอาจารย์ (จาก `usersAPI.listScopusUsers`, **field ชื่อ `scopus_id`**)
- dropdown ช่วงปี **แสดง พ.ศ.** แต่เก็บ/ส่งค่าเป็น **ค.ศ.** (backend ใช้ ค.ศ.); option = ปีที่มี doc จริง
- default = ช่วงปีเต็มที่มีข้อมูล (h-index เป็นค่าสะสมทั้งอาชีพ — ถ้า default ตาม filter 3 ปีจะได้ h≈1 ซึ่งดูพัง)
- กราฟ ApexCharts (area citations + เส้น y=x + จุด h + เส้นประที่ h); ปิด drag-zoom
- ปุ่ม export 3 ปุ่ม:
  - **"ส่งออก CSV (ทั้งหมด)"** (หัวการ์ด) → `getAuthorHIndexSummary()` → CSV ทุกคน
  - **"ส่งออกบทความ (CSV)"** (รายบุคคล) → CSV รายการบทความของคนที่เลือก จาก `graph.points`
  - **"ส่งออกรายงาน (พร้อมกราฟ)"** → ไฟล์ **HTML** ฝังภาพกราฟ (PNG จาก `ApexCharts.exec(id,"dataURI")`) + ตาราง (เปิด/พิมพ์ PDF ได้)
- CSV ใช้ Blob + BOM (`﻿`) ให้ Excel อ่านไทยได้ (mirror `exportPersonSummaryCSV` ของ Person Summary)

---

## 4. Design decisions & gotchas (อ่านก่อนแก้)

### 4.1 คอลัมน์ CSV export ทั้งหมด
`ลำดับ · รหัสอาจารย์ · ชื่อ-สกุล · Scopus Author ID · h-index · จำนวนเอกสาร · การอ้างอิงรวม · ผู้เขียนร่วม · ช่วงปีผลงาน(พ.ศ.)`
- ใช้ **h-index คอลัมน์เดียว** (ค่าคำนวณ = ตรงกับกราฟ) — เดิมมี 2 คอลัมน์ (ในระบบ/Scopus) ทำให้ผู้ใช้งง เพราะปกติเท่ากัน
- endpoint `/summary` ยังคืน field ทางการ (`scopus_h_index`, `scopus_cited_by_count`, ...) มาด้วย เผื่ออยากเพิ่มคอลัมน์ภายหลัง

### 4.2 GORM column mapping — EID
`AuthorHGraphService.GetGraph` docRow **ต้องมี** `gorm:"column:eid"` ที่ field `EID`
เพราะ GORM naming strategy แปลง `EID` เป็นชื่อคอลัมน์ไม่ตรงกับ alias `eid` → ค่าจะว่างถ้าไม่ระบุ tag

### 4.3 Lock fix — RELEASE_LOCK NULL (กระทบหลาย subsystem)
`RELEASE_LOCK` คืน **NULL** ถ้า lock ไม่มีอยู่แล้ว (connection ที่จับ lock ถูก pool รีไซเคิลระหว่างงานยาว → auto-release)
การ scan NULL ลง `int` ทำให้ error `"converting NULL to int is unsupported"` ตอนจบ batch import ยาว ๆ
**แก้:** รวมเป็น helper `releaseNamedLock(ctx, db, lockName)` (`services/named_lock.go`) scan เป็น `sql.NullInt64`, best-effort ไม่ error
**ใช้แทน pattern เดิมทั้งหมด 9 จุด:** citescore / kku_people / scholar / scopus_author_metrics / scopus_conference / scopus_ingest_job / scopus_benchmark_harvest / thaijo_ingest_job / thaijo_ingest_service
(เป็น bug เดิมของหลาย subsystem ไม่ใช่เฉพาะฟีเจอร์นี้ — แก้ยกชุดเพื่อความสม่ำเสมอ)

### 4.4 พ.ศ. ↔ ค.ศ.
backend เก็บ/รับ-ส่งเป็น **ค.ศ.** ทุกที่ (จาก `YEAR(cover_date)`); frontend แสดงเป็น **พ.ศ.** (`ce + 543`) เท่านั้น
filter ปีด้านบนของ dashboard ก็เป็น พ.ศ. — อย่าสับสน

### 4.5 ข้อจำกัด Scopus Author API
- ต้องยิงจาก **IP สถาบัน / VPN** ไม่งั้น **401** (per-request log จับ 401 ไว้แล้ว)
- โควตา **5,000 คำขอ/สัปดาห์**, ยิงทีละคน (ไม่มี batch endpoint) → เหมาะรันเดือนละครั้ง (h-index ขยับช้า)

### 4.6 h-graph จาก DB อาจต่ำกว่า scopus.com
`scopus_documents.citedby_count` เป็นค่า ณ ตอน ingest → ถ้าเก่า h-index ที่คำนวณจะต่ำกว่าเว็บ Scopus
แก้โดยรัน **scopus document ingest** (`scopus-ingest` / ปุ่ม import scopus) ใหม่ให้ citation สด

---

## 5. การใช้งาน / operate

1. ตั้ง Scopus API key ใน `scopus_config` (key `X-ELS-APIKey`) — ใช้ร่วมกับ subsystem scopus อื่น
2. ต่อ VPN/IP สถาบัน
3. ดึง h-index: กดปุ่ม "ดึง h-index ทุกคน" ในหน้า academic-imports **หรือ** `go run ./cmd/scopus-author-metrics`
4. ดูกราฟ/ส่งออก: หน้า research-dashboard เลื่อนลงล่างสุด → การ์ด h-index

> **สำคัญ:** backend dev บน `:8080` **ไม่มี auto-reload (ไม่มี air)** — โค้ด Go ใหม่ต้อง restart process เอง

---

## 6. Known limitations / follow-ups (ยังไม่ได้ทำ)
- ยังไม่ตั้ง **schedule** รัน `scopus-author-metrics` อัตโนมัติ (แนะนำเดือนละครั้ง) เพื่อสะสม snapshot ทำกราฟ h-index-ตามเวลา
- ยังไม่มีหน้าแสดง **กราฟ h-index ตามเวลา** จาก snapshot (ข้อมูลเริ่มสะสมแล้วในตาราง แต่ยังไม่มี UI)
- ยังไม่ผูก h-index เข้ากับ Person Summary / ตาราง ranking หลักของ dashboard
- export "ทุกคน" ต้องมี `scopus_author_metrics` snapshot ถึงจะมีคอลัมน์ "ผู้เขียนร่วม" (ไม่มีก็เว้น "-")
