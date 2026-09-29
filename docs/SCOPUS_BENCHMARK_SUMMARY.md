# สรุปผลงานและบทบาทอาจารย์ใน Scopus Benchmark

เริ่ม 2026-09-30; branch `feat/scopus-benchmark-summary` จาก main ทั้งสอง repo

## แหล่งข้อมูล

หน้า `/admin/scopus-benchmark` เพิ่มแท็บแรก “สรุปผลงานและบทบาทอาจารย์” ตามด้วย “ผลเปรียบเทียบเชิงวิเคราะห์” เดิม และ “ตั้งค่า & ดึงข้อมูล” ใช้ผล AI/XML ที่มีแล้ว ไม่มี Scopus request, AI หรือ backfill ระหว่างเปิดรายงาน

ฐานเดียวคือ benchmark documents ที่มี membership ปีรายงานใน scope `country_thailand` (country/Thailand) ทุกระดับใช้ตัวกรองเดียวกัน นับ EID ไม่ซ้ำ ไม่เติม core documents ที่อยู่นอกฐาน:

| ระดับ | เงื่อนไข |
| --- | --- |
| Thailand | ชุดฐานหลังกรอง |
| KKU | มี document AF-ID 60017165, 60280609, 60026046, 60277695 หรือ 109899034 |
| COC | ภายใน KKU มีผู้เขียนตรงทะเบียนอาจารย์ และผู้เขียนนั้นมี AF-ID 60017165 หรือ 60280609 |

COC คือ College of Computing สังกัดของผู้เขียนคนอื่นไม่ทำให้อาจารย์เข้าเกณฑ์ ทะเบียนใช้ `users.delete_at IS NULL`, `is_test=0`, `role_id IN (1,4,5)` จับคู่ Scopus Author ID ปัจจุบัน ไม่ใช้ `is_faculty` เก่าหรือวันเริ่มงาน

Category/confidence อ่านจาก benchmark; ชื่อหมวดจาก `paper_categories` ไม่คัดลอก classification จาก core ค่าเริ่มต้นปีปัจจุบัน/ปีก่อนหน้า (ค.ศ.), Journal, มี Category, confidence High/Medium/Preface/unknown (ตัดเฉพาะ Low) และแยก T1

## สูตรและสถานะ

- `%KKU = KKU/Thailand×100`; `%COC = COC/KKU×100` รวมจากจำนวนรวม ไม่เฉลี่ยเปอร์เซ็นต์ ตัวหาร 0 คืน null/ขีด
- ปี `missing` คืนจำนวน null ทุกระดับ ปีที่มีข้อมูลแต่กรองไม่พบคืน 0 รวมช่วงที่ขาดบางปีเขียน “รวมจากปีที่มีข้อมูล”
- `expected` มาจาก count snapshot เพื่อแจ้งความครบถ้วน ไม่ใช้เป็นยอดรายงาน มี snapshot แต่ยังไม่มี membership/harvest ไม่ถือว่าพร้อม
- `available`: มี harvest สำเร็จครอบคลุมปี; `partial`: มี membership แต่ยังยืนยันความครบถ้วนไม่ได้/น้อยกว่า expected; `harvesting`: กำลังดึง; `missing`: ยังไม่มีชุดเอกสาร
- metadata scope subject/extra query และเวลา harvest อยู่ใน year_states ต้องตรวจช่วง/ขอบเขตให้ตรงความต้องการ ข้อมูล dev ไม่แสดงสถานะ production

Quartile ตรง research-dashboard: source ID + `doc_type=all` + Complete ปีตีพิมพ์ก่อน หากไม่มีเลือก Complete ล่าสุด **ก่อน** ปีตีพิมพ์ ไม่ใช้ InProgress/อนาคต รายละเอียดมี metric_year/fallback แยก T1 เมื่อ percentile 90–100 และไม่ซ้ำ Q1–Q4; toggle Q1–Q4 กลับใช้ Quartile ของ metric เดิม cohort/ยอดรวมไม่เปลี่ยน Journal ไม่มี Quartile → missing; non-Journal → not_applicable

## บทบาทและหน่วยนับ

จับคู่ benchmark/core ด้วย EID และ Author ID อ่าน `scopus_document_authors` flags บน `scopus_documents.author_role_status` ไม่อนุมาน seq ใน benchmark ไม่คัดลอก roles ไป benchmark

รู้บทบาทเมื่อสถานะ complete/no_correspondence และ flags ทั้งสองไม่ว่าง: First/Corresponding ตาม flags (ซ้อนกันได้/หลาย corresponding ได้); Lead = union; Co = สอง flags false; missing EID/Author ID, flags ว่าง, pending/needs_review/fetch_error = unknown

no_correspondence ใช้กติกาที่ผู้ใช้ตกลงไว้ ไม่ใช่หลักฐานว่าไม่มี corresponding จริง ยังไม่แยก co-first/co-corresponding หลักฐาน XML และข้อจำกัดอยู่ `SCOPUS_AUTHOR_ROLES.md`

รายบุคคลนับ `(user_id,EID)` เฉพาะผู้เขียนที่ผ่าน COC สัดส่วนหารผลงานทั้งหมดหลังกรองรวม unknown ผลรวมรายคนอาจมากกว่า papers ไม่ซ้ำ ไม่มี Scopus ID แสดงเชื่อมไม่ได้ มี ID แต่ไม่พบแสดง 0 จากชุดที่สังเกตได้

ภาพรวม First/Corresponding = มีอาจารย์เข้าเกณฑ์อย่างน้อยหนึ่งคน จึงซ้อนกันได้ Lead/Co-only/Unknown แบ่ง papers ไม่ซ้ำ: มี lead → lead; ไม่มี lead และมี unknown → unknown; ไม่มี lead และทุกคน known-co → co-only บทบาทไม่ใช่คะแนนปริมาณงานที่ทำจริง รายละเอียดแสดงผู้เขียนทุกคนรวมภายนอกคณะ จึงเห็นคนที่ first/corresponding แม้อาจารย์เราเป็น co

## Schema และ ingest

Migration `049_20260930_scopus_benchmark_summary.sql` แบบ rerunnable เพิ่ม fields ที่ขาด ไม่ reset classification/taxonomy เดิม:

- benchmark classification fields: category/confidence/model/taxonomy_version/classified_at
- `affiliations_complete` ใน benchmark document และ document-author
- `scopus_benchmark_document_affiliations`: PK document_id/afid
- `scopus_benchmark_author_affiliations`: PK document_id/author_id/afid
- provenance = benchmark_payload/core_payload/legacy_first

เก็บ AF-ID ตรง จึงไม่พลาดกรณีไม่มีชื่อใน catalogue คง affiliation_id ตัวแรกให้ระบบเก่า Harvest normalize ทุก AF-ID จาก payload ปรับรายการที่ได้รับใหม่ ตรวจ author-count ก่อนลบ roster ที่หายไป ข้อมูลขาดไม่ถือเป็นรายการเต็ม `preserveBenchmarkClassification` รักษา classification ทั้งหมดเมื่อ Save ซ้ำ ไม่เขียน core role tables

ระบบเดิมไม่เก็บ benchmark raw JSON ใหม่หลัง migration 042 จึง normalize ขณะ ingest ก่อนทิ้ง payload ต้องลง 049 **ก่อน deploy backend** ที่อ่าน/เขียนคอลัมน์ใหม่

## Backfill และการเปิดใช้

อ่าน batch 100, named harvest lock บน pinned connection Seed legacy-positive เป็นชุด แล้ว commit payload รายผลงาน หยุดแล้วรันซ้ำได้:

1. benchmark raw JSON ที่ EID ตรง → ทุก AF-ID
2. core raw JSON ที่ EID ตรง → เติม metadata ที่ขาด เฉพาะ Author ID ที่ตรงและยังไม่ complete ไม่ทับ benchmark full metadata
3. ไม่มี JSON → legacy first เป็นหลักฐานเชิงบวกเท่านั้น ไม่ยืนยันว่าไม่มีสังกัดอื่น

ไม่เปลี่ยน classification, roles หรือ membership ไม่ยิง Scopus การรันซ้ำอาจยังอ่าน payload ที่ไม่ครบแต่ไม่เปลี่ยนผล หาก raw ถูกล้างและ core ไม่มี ต้อง harvest country scope/ช่วงปีที่ต้องการใหม่หลัง deploy normalizer ผ่านแท็บตั้งค่า แล้วจัด Category ด้วย workflow ที่ใช้อยู่ ไม่ harvest อัตโนมัติเมื่อเปิดรายงาน

คำสั่งใช้ `.env` ของ dev และบังคับชื่อ DB ตรงเพื่อป้องกันเขียนผิดเป้าหมาย ไม่พิมพ์ key/password:

```powershell
go run ./cmd/scopus-benchmark-summary -migrate migrations/049_20260930_scopus_benchmark_summary.sql -expect-database <DEV_DATABASE>
go run ./cmd/scopus-benchmark-summary -backfill -expect-database <DEV_DATABASE>
go run ./cmd/scopus-benchmark-summary -audit -year-from 2025 -year-to 2026
go run ./cmd/scopus-benchmark-summary -verify-migration migrations/049_20260930_scopus_benchmark_summary.sql -expect-database <DEV_DATABASE>
go run ./cmd/scopus-benchmark-summary -backfill -verify-backfill -expect-database <DEV_DATABASE>
```

verify-migration ใช้ตาราง fixture แยกชื่อ สร้าง schema ใหม่/seed แล้วรันซ้ำ ตรวจ classification/taxonomy ที่แก้เองยังอยู่ และล้างเฉพาะตารางที่สร้าง verify-backfill ใช้หลังรอบแรกเสร็จแล้ว เทียบ revision กับ fingerprint count/CRC ของ classification และ core role rows งด jobs เขียนอื่นขณะตรวจ

Production ขั้นตอน: สำรอง DB → ตรวจ target/schema → ลง 049 → deploy backend → backfill ด้วย environment ที่ตรวจแล้ว → audit coverage → deploy frontend ไม่มีการแก้ production อัตโนมัติ ยอด dev ไม่จำเป็นต้องเท่ากับ Excel production

## API และ Excel

ใต้ `/api/v1/admin/scopus/benchmark/summary`, สิทธิ์ `scopus.publications.read` หรือ `ui.page.admin.scopus.view`:

| GET | ข้อมูล |
| --- | --- |
| `/options` | ปี/Category/ประเภท/confidence |
| root (ไม่มี trailing slash) | yearly/categories/quartiles/faculty_roles |
| `/faculty` | roster ครบ, counts/ratios |
| `/documents` | details+ผู้เขียนทุกคน, 50 ต่อหน้า |
| `/export` | XLSX overview/faculty |

Filters: year_from/year_to (1900 ถึงปีปัจจุบัน+1 สูงสุด 60 ปี), types CSV หรือ all, category=classified/all/unknown/ID, confidence CSV High/Medium/Low/Preface/unknown, quartile_mode=t1/q

Details เพิ่ม level=thailand/kku/coc, year, document_category (0=ไม่มี), quartile, role=first/corresponding/lead/co/unknown, user_id, page ใช้ filter กลางเดียวกัน

ทุก report response มี applied_filters/generated_at/revision/year_states/coverage อ่าน streams ใน read-only REPEATABLE READ snapshot เดียว คำนวณบน backend ตามช่วงปี ส่งเฉพาะ aggregate หรือหน้ารายละเอียด ไม่ส่ง country dataset ไป browser

Excel ใช้ excelize เก็บ counts/percentages numeric มีชีตคำอธิบาย filters/formula/revision/time/coverage ภาพรวมมีรายปี/Category/Quartile/บทบาทคณะ; faculty ส่งทะเบียนหลัง applied filters ทั้งหมด Search/hide/sort ฝั่ง UI เป็น display-only ไม่ลด cohort Excel

Export บังคับ view+revision สร้าง snapshot ใหม่และเทียบ filters/selected documents/roles/affiliations/metrics/roster/taxonomy/year states/coverage ถ้าเปลี่ยนคืน 409 ให้ refresh ก่อน ไม่มีไฟล์ยอดต่างจากผลที่เห็น

## Lazy loading และรายงานเดิม

เปิดหน้าเรียก options/summary เท่านั้น; faculty เมื่อเปิด subview; details เมื่อกดจำนวน แท็บวิเคราะห์ mount เมื่อเคยเปิด หยุด comparison/insights เมื่อ inactive ใช้ cache เมื่อกลับ Setup เริ่ม scopes/runs/comparison เมื่อเปิด setup; งานที่เริ่มไว้ polling runs เบาๆ เพื่อแจ้ง stale หลังจบ

Setup writes/harvest ทำเครื่องหมาย cache เก่า แสดง refresh ไม่ reload รายงานทุกแท็บ มี AbortController และ generation กัน response เก่าทับ filters ใหม่

แท็บวิเคราะห์ยังใช้ scopes/snapshots/ความพร้อมและ employment refinement เดิม ยอดอาจต่างจากรายงานใหม่ฐาน Thailand/AF-ID 5/2/ไม่กรองวันเริ่มงาน ห้ามเติม core ที่อยู่นอกฐานเพื่อทำยอดให้เท่ารายงานเก่า

## ผลตรวจ dev 2026-09-30

- Migration บน dev เดิม/รันซ้ำและ fresh fixture ผ่าน Classification/taxonomy ที่แก้เองยังอยู่
- Backfill ตรวจ 5,172 papers: benchmark payload 0, core payload ใช้ในรอบแรก 224, legacy-only 4,948, invalid 0 ตัวเลข payload เป็นจำนวนใช้ในรอบนั้น ไม่ใช่ metadata สะสม
- รันซ้ำ: core payload ที่ยังต้องอ่าน 2, legacy-only 5,170; revision คงเดิม; classification/core XML role fingerprints ไม่เปลี่ยน
- Thailand 2025 ไม่มี membership แต่ expected 5,597 → missing/ขีด; 2026 base 3,344 Category 0 → default report ผ่าน filters 0
- Journal/Category ทั้งหมด/non-Low: Thailand 2,158 → KKU 163 → COC 37; First 4, Corresponding 21, Lead 22, Co-only 14, Unknown 1
- หลัง backfill selected doc affiliations incomplete 2,122; faculty-author affiliations incomplete 1; unknown role 1 คู่; metric fallback 1,722; missing Quartile 436; faculty ไม่มี ID 4
- ทั้งหมดเป็น dev ไม่ใช่ production และไม่ได้แก้หมวดให้เลียนแบบ Excel

## การตรวจสอบ

Backend `go test ./...`: cohort AF-ID 5/2, second affiliation/other-author KKU, multiple faculty pairs, overlapping/multiple corresponding, no_correspondence/nil/pending flags, filters/missing year/zero denominator, Complete fallback/T1 toggle, revision, ingest classification preservation, normalized relations/reconcile และ truncated payload, XLSX อ่านกลับตรวจ numeric values/revision

Frontend node tests+production build และ development harness `/dev/scopus-benchmark-summary` ตรวจ API calls ตามแท็บ/cache, tooltip/toggle/search/sort/hide/details ผู้เขียนภายนอก/pagination/filters/reset/refresh/revision error ข้อมูล harness สมมติและ route 404 ใน production

ผลตรวจ UI จริงผ่าน harness: เปลี่ยนแท็บไม่เพิ่ม request ที่ cache แล้ว, draft/apply/reset/refresh, tooltip/toggle, faculty search/hide, details pagination และผู้เขียนนอกคณะ, missing year, API error, Excel revision error, รวมทั้ง stale หลัง setup write คงอยู่จนกด refresh จริง Frontend tests 67 ข้อผ่าน; backend go test ./... ผ่าน
