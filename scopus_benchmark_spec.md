# Scopus Benchmark (CS: faculty vs KKU vs Thailand)

เปรียบเทียบจำนวนผลงาน **Computer Science (SUBJAREA COMP)** ระดับ **คณะ / มหาวิทยาลัย (KKU) / ประเทศ (Thailand)**
โดยเก็บข้อมูลแบบละเอียด (raw ครบ) ใน **ตารางแยกชุดใหม่** ที่ไม่แตะ `scopus_documents` เดิม

## ทำไมต้องแยกตาราง
`unified_search_contents` / `unified_search_authors` (migrations 013/014/023), public publication search/detail
และ admin `ListAll` (`services/scopus_publication_service.go` `ListAll`) อ่าน `scopus_documents` **ทั้งตารางโดยไม่ filter user**
และ stamp เป็น `'faculty'` → ถ้าใส่ docs ระดับ KKU/Thailand เข้าไป จะรั่วเข้า public search + over-count
จึงเก็บใน `scopus_benchmark_*` แยกทั้งหมด (dashboard คณะเดิมปลอดภัยอยู่แล้วเพราะ join ผ่าน `users.scopus_id`)

## ตาราง (migration 029)
- `scopus_benchmark_scopes` — ทะเบียน scope (`university_kku` มี `af_id`, `country_thailand` มี `affil_country`)
- `scopus_benchmark_documents` — mirror `scopus_documents` + `raw_json` (view=COMPLETE) · unique `eid`
- `scopus_benchmark_authors` / `scopus_benchmark_document_authors` — author list (COMPLETE) + `is_faculty`
- `scopus_benchmark_document_scopes` — membership doc↔scope + `pub_year`
- `scopus_benchmark_harvest_runs` — ประวัติการรัน (+ `cursor_state` resume)
- `scopus_benchmark_count_snapshots` — จำนวนต่อ (scope, ปี) ณ เวลาหนึ่ง

## นิยามการเทียบ
- **country** = `AFFILCOUNTRY(Thailand) AND SUBJAREA(COMP)`
- **university** = `AF-ID(<KKU>) AND SUBJAREA(COMP)`
- **faculty (derived)** = docs ใน university harvest ที่มี author `authid` ตรงกับ `users.scopus_id` (`is_faculty=1`)
  → CS-consistent, เซตซ้อน `faculty ⊆ university ⊆ country`, ไม่ต้องยิง query เพิ่ม

## Scopus API ที่ใช้
- Affiliation Search: `GET /content/search/affiliation?query=AFFIL(<name>)` → หา AF-ID
- Search (count): `GET /content/search/scopus?query=<...>&count=1` → อ่าน `opensearch:totalResults`
- Search (harvest): `view=COMPLETE`, `count=25`, **cursor pagination** (`cursor=*` → อ่าน `search-results.cursor.@next`)
  ทะลุ limit 5,000 ต่อ query · quota 20,000 req/7 วัน · 429 → backoff
- API key อ่านจาก `scopus_config` (`X-ELS-APIKey`) — ต้องอยู่ใน VPN KKU

## Endpoints (admin, guard: `ui.page.admin.scopus.view`)
| Method | Path | หน้าที่ |
|---|---|---|
| POST | `/admin/scopus/benchmark/affiliation/lookup` | หา AF-ID |
| GET/PUT | `/admin/scopus/benchmark/scopes[/:id]` | จัดการ scope |
| POST | `/admin/scopus/benchmark/counts/refresh?years_back=N` | อัปเดตตัวเลข + snapshot |
| POST | `/admin/scopus/benchmark/harvest` | harvest (async 202, 409 ถ้ารันค้าง) |
| GET | `/admin/scopus/benchmark/runs` | ประวัติการรัน |
| GET | `/admin/scopus/benchmark/comparison?years_back=N` | ข้อมูลเทียบรายปี |

## Cron (optional) — `cmd/scopus-benchmark`
```
scopus-benchmark -counts-only                          # refresh counts ทุก active scope
scopus-benchmark -scope university_kku -years-back 10   # harvest KKU ย้อนหลัง 10 ปี
scopus-benchmark -scope country_thailand                # harvest Thailand ทุกปี
```

## Deploy checklist
1. รัน migration `029` บน prod DB (สร้างตาราง benchmark + seed 2 scope)
2. ตั้ง AF-ID ของ KKU ผ่าน affiliation lookup (หน้า admin) — ปัจจุบัน `60017165`
3. harvest `university_kku` (จำเป็นต่อการ derive คณะ) แล้วค่อย `country_thailand`
4. (optional) ตั้ง cron เดือนละครั้ง

## ตัวเลขอ้างอิง (ยิงจริง ก.ค. 2026)
KKU CS ทั้งหมด ≈ 2,501 · Thailand CS ทั้งหมด ≈ 53,736 · ปี 2024: คณะ 50 / KKU 231 / Thailand 4,732
