# Scopus Benchmark — Handoff / คู่มือ

> เอกสารส่งต่องาน (สำหรับคนและ AI agent) — ระบบ **เทียบผลงาน Scopus หมวด Computer Science ระดับ คณะ vs มหาวิทยาลัย (KKU) vs ประเทศ (Thailand)**
> รายละเอียดเชิงเทคนิคเพิ่มเติมดู [`scopus_benchmark_spec.md`](./scopus_benchmark_spec.md)

สถานะล่าสุด: **ยังไม่ merge เข้า main** — อยู่บน branch `feature/scopus-benchmark` (ทั้ง backend และ frontend)

---

## 1. เป้าหมายของงาน

เดิมระบบดึง Scopus แค่ **ระดับคณะ** (วนตาม `users.scopus_id`) เก็บใน `scopus_documents`
งานนี้เพิ่มการดึง/นับผลงาน **หมวด Computer Science (SUBJAREA COMP)** ในระดับที่กว้างขึ้น เพื่อ **เทียบส่วนต่าง**:

```
คณะ (faculty)  ⊆  มหาวิทยาลัย KKU  ⊆  ประเทศ Thailand      (ทั้งหมดกรอง SUBJAREA COMP)
```

## 2. การตัดสินใจเชิงสถาปัตยกรรม (สำคัญ อย่าทำผิดซ้ำ)

**ห้ามเก็บ docs ระดับ KKU/Thailand ลงตาราง `scopus_documents` เดิมเด็ดขาด**
เพราะ view `unified_search_contents`/`unified_search_authors` (migrations 013/014/023), public publication search/detail
และ admin `ListAll` (`services/scopus_publication_service.go` → `ListAll`) อ่าน `scopus_documents` **ทั้งตารางโดยไม่ filter user**
แล้ว stamp เป็น `'faculty'` → ถ้าปนเข้าไปจะ **รั่วเข้า public search + over-count ทันที**

→ จึงเก็บใน **ตารางแยกชุดใหม่ prefix `scopus_benchmark_`** ทั้งหมด (dashboard คณะเดิมปลอดภัยอยู่แล้วเพราะ join ผ่าน `users.scopus_id`)

## 3. นิยามการเทียบ

ทุก scope intersect กับ `SUBJAREA(COMP)`:
- **country** = `AFFILCOUNTRY(Thailand) AND SUBJAREA(COMP)`
- **university** = `AF-ID(<KKU_AFID>) AND SUBJAREA(COMP)`  (KKU = **60017165**)
- **faculty** = `AF-ID(<KKU>) AND SUBJAREA(COMP) AND (AU-ID(a) OR AU-ID(b) ...)` โดย `a,b,...` = `users.scopus_id`
  ของอาจารย์ในระบบ → นับด้วย **author-set count** (scope `faculty_cs`, level=`faculty`, migration 034)

**ทั้งสามคอลัมน์** มาจาก **count query** (`count=1` → `opensearch:totalResults`) เก็บเป็น snapshot รายปี — เร็ว ไม่ต้องดึงเอกสารจริง
กด "อัปเดตตัวเลข" ครั้งเดียวเติมครบทั้ง คณะ/KKU/Thailand

> **⚠️ API key entitlement (สำคัญ):** key ปัจจุบันถูกลดสิทธิ์ ใช้ได้แค่ **STANDARD view** —
> `cursor` และ `view=COMPLETE` ถูกจำกัด (403/401) จึง (1) ใช้ **offset pagination** แทน cursor
> และ (2) เปลี่ยนการนับคณะเป็น **author-set count** แทนการ derive จาก COMPLETE harvest
> การ **harvest เอกสารเต็ม** (ต้องใช้ COMPLETE + author list) จึง**ใช้ไม่ได้จนกว่าจะได้สิทธิ์คืน** — เป็น optional/ขั้นสูง

## 4. โครงสร้างข้อมูล (migration `033_..._create_scopus_benchmark_tables.sql` + `034_..._add_faculty_benchmark_scope.sql`)

| ตาราง | หน้าที่ | key |
|---|---|---|
| `scopus_benchmark_scopes` | ทะเบียน scope (seed: `university_kku`, `country_thailand`) | code |
| `scopus_benchmark_documents` | เอกสาร mirror `scopus_documents` + `raw_json` (COMPLETE) | eid unique |
| `scopus_benchmark_authors` | ผู้เขียน (จาก author list ของ COMPLETE) | scopus_author_id unique |
| `scopus_benchmark_document_authors` | ลิงก์ doc↔author + `is_faculty` | (document_id, author_id) |
| `scopus_benchmark_document_scopes` | membership doc↔scope + pub_year | (document_id, scope_id) |
| `scopus_benchmark_harvest_runs` | ประวัติ run (+ `cursor_state` resume, status: running/success/failed/cancelled) | |
| `scopus_benchmark_count_snapshots` | จำนวนต่อ (scope, ปี) ณ เวลาหนึ่ง | |

> หมายเหตุ: run tables ทั้งโปรเจกต์สร้างแบบ out-of-band — migration 033 ต้องรันบน DB จริงเองตอน deploy

## 5. โค้ดหลัก (backend `fund-management-api`)

- `models/scopus_benchmark.go` — models ทั้งหมด
- `services/scopus_benchmark_service.go` — API client: `ResolveAffiliation`, `CountScope`, `searchPage` (cursor), `DetectYearRange` (sort `±coverDate`)
- `services/scopus_benchmark_harvest.go` — `HarvestScope` (cursor loop, upsert, is_faculty, GET_LOCK `scopus_benchmark_harvest_lock`, 429 backoff, cancel check ต่อหน้า)
- `controllers/admin_scopus_benchmark_controller.go` — endpoints
- `routes/routes.go` — ลงทะเบียนใต้ admin group (permission `ui.page.admin.scopus.view`)
- `cmd/scopus-benchmark/main.go` — cron binary (`-scope`, `-years-back`, `-counts-only`)
- reuse helper จาก `scopus_ingest_service.go`: `scopusEntry`, `parseScopusDate`, `extractStringFromRaw`, `cloneJSON`, `lookupScopusAPIKey` (ไม่แตะ ingest คณะเดิม)

### API endpoints (prefix `/api/v1/admin/scopus/benchmark`)
| Method | Path | หน้าที่ |
|---|---|---|
| POST | `/affiliation/lookup` | หา AF-ID |
| GET | `/scopes` | list scope |
| PUT | `/scopes/:id` | แก้ scope (เช่นตั้ง af_id) |
| GET | `/scopes/:id/year-range` | ตรวจปีแรก/ปีล่าสุด |
| POST | `/counts/refresh?year_from&year_to` (หรือ `years_back`) | นับ + snapshot |
| POST | `/harvest` (body: scope_id, year_from/year_to หรือ years_back) | harvest async (202, 409 ถ้ามี run ค้าง) |
| GET | `/runs` | ประวัติ run |
| POST | `/runs/:id/cancel` | ยกเลิก run (หยุดภายใน 1 หน้า / เคลียร์ run ค้าง) |
| GET | `/comparison?year_from&year_to` | ข้อมูลเทียบรายปี |

## 6. โค้ดหลัก (frontend `frontend_project_fund`)

- หน้า: `app/(portal)/research-fund-system/admin/components/research/AdminScopusBenchmark.js`
- API client: `app/lib/api.js` → `scopusBenchmarkAPI`
- เสียบเข้า dispatcher/menu: `admin/page.js`, `app/lib/admin_menu_config.js`, `admin/components/layout/Navigation.js` (page id = `scopus-benchmark`)
- UI: ตารางเทียบรายปี, โหมด **ช่วงปี (range) เป็นหลัก** + toggle "ย้อนหลัง X ปี", ปุ่ม "ตรวจปีแรก (จาก KKU)", affiliation lookup, harvest + run controls (banner ตอนรัน, disable ปุ่มตอนมี run ค้าง, auto-refresh ทุก 5 วิ, ปุ่มยกเลิก)

## 7. วิธีรัน (dev)

ต้องอยู่ใน **VPN KKU** (API เรียก Scopus) และ DB config ใน `fund-management-api/.env`
```bash
# backend  (entrypoint = cmd/api ไม่ใช่ root)
cd fund-management-api && go run ./cmd/api        # :8080

# frontend
cd frontend_project_fund && npm run dev           # :3000
```
เข้าเมนู admin → "เทียบผลงาน Scopus (CS)" (permission `ui.page.admin.scopus.view`)

**flow ใช้งานที่ถูกต้อง:**
1. ตั้ง AF-ID ของ KKU (seed มี scope แล้ว, af_id = 60017165 — ตั้งผ่าน affiliation lookup ได้)
2. **Harvest scope `university_kku`** (เลือกช่วงปี) → ได้คอลัมน์ "คณะ" + เป็นฐานข้อมูล
3. กด **"อัปเดตตัวเลข KKU/Thailand"** → เติมตัวเลข 2 คอลัมน์นั้นให้ครบทุกปี (เร็ว)
4. (optional) harvest `country_thailand` ถ้าต้องเก็บเอกสารเต็ม

## 8. ตัวเลขอ้างอิง (ยิงจริง)

KKU CS ทั้งหมด ≈ 2,501 · Thailand CS ≈ 53,736 · **KKU CS เริ่มปี 1981**, Thailand เริ่ม 1965
ปี 2024: คณะ 50 / KKU 231 / Thailand 4,732

## 9. สถานะ Git & สิ่งที่เหลือ

- **branch:** `feature/scopus-benchmark` (backend 6 commits ถึง `ede6041`, frontend 2 commits ถึง `f4c675c`) — push origin แล้ว **ยังไม่ merge main**
- **main เคลื่อนไปไกล:** backend main นำหน้า ~20 commits, frontend ~26
- **conflict คาดว่ามีไฟล์เดียว:** backend `routes/routes.go` (frontend ไม่มี overlap)

### TODO ก่อน merge ขึ้นจริง
1. **Sync feature branch กับ main ล่าสุด** (merge/rebase) — แก้ conflict `routes/routes.go`
2. **rebuild + restart backend** — bug ที่เจอครั้งก่อน (route 404) เกิดจาก backend ที่รันเป็น build เก่า ไม่ใช่บั๊กโค้ด
3. **ตรวจ DB** — ตาราง benchmark (migration 033) อาจต้องรันซ้ำถ้า intern DB ถูก reset
4. **verify ใน browser จริง** — ที่ทำไปคือ parse + เช็ก route (401) ยังไม่เคยคลิกทั้ง flow หลัง login
5. **(UX) ปุ่ม "ตรวจปีแรก"** โผล่เฉพาะโหมดช่วงปี — ผู้ใช้เคยบอกว่างง อาจย้ายให้เห็นตลอด/เปลี่ยนชื่อ
6. deploy: รัน migration 033 + 034 บน prod + ตั้ง af_id KKU + (optional) cron

## 10. Gotchas สำหรับ agent

- **อย่าแตะ `scopus_documents` / unified views / faculty ingest** — งานนี้ต้อง isolated
- Scopus Search cap 5,000/query → ใช้ cursor (`cursor=*` → `search-results.cursor.@next`)
- view=COMPLETE จำกัด 25/หน้า · sort ascending ต้องใช้ `+coverDate` (ไม่มี prefix = descending)
- quota 20,000 req/7 วัน · 429 → มี backoff แล้ว
- ตอนทดสอบกับ intern DB **ดึงแค่ตัวอย่างเล็ก** อย่า harvest ทั้ง Thailand ลง DB ที่ไม่ใช่ prod
