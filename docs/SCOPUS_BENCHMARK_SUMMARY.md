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

Category/confidence อ่านจาก benchmark; ชื่อหมวดจาก `paper_categories` ไม่คัดลอก classification จาก core ค่าเริ่มต้นปีปัจจุบัน/ปีก่อนหน้า (ค.ศ.), Journal, มี Category, confidence High/Medium/unknown (ตัด Low และ Preface) และแยก T1

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

### สรุปเปรียบเทียบ Thailand / KKU / COC

แท็บหลักใหม่อยู่ขวาของสรุปผลงานและบทบาทอาจารย์ ใช้ filters เดิมที่แยก state ใน frontend ขอ `GET summary?report_view=presentation` เพื่อเพิ่ม `presentation.steps/categories/quartiles` จาก snapshot เดียว ไม่เปลี่ยน API เดิมที่ไม่ส่ง report_view ไม่เพิ่ม schema/Scopus requests

`scopus_benchmark_presentation.go` เตรียม membership KKU/COC ก่อนกรองด้วย AF-ID และทะเบียนเดียวกับรายงานเดิม EID ซ้ำไม่นับเพิ่ม จำกัด membership ให้อยู่ในช่วงปีที่ขอ ขั้นตอนสะสม: ฐานก่อนกรอง → ประเภท → Category → Confidence ตาม filters ที่ผู้ใช้ใช้จริง; %คงเหลือ = ยอดขั้นนั้น / ยอดก่อนกรองของระดับนั้น ×100; nil/ตัวหาร 0 คืน nil ยอดท้ายเท่ากับ total/รายปี/Category/Quartile ขั้นที่เลือกทั้งหมดอาจไม่เปลี่ยนจำนวน

แต่ละ step มี filters ที่ใช้สร้าง step นั้นสำหรับ drilldown ผ่าน documents endpoint ตามเดิม โดยใช้ types=all/category=all/confidence ทุกค่าที่ระบบรองรับในขั้นก่อนการกรองส่วนนั้น ไม่ผูก step drilldown กับผลหลังกรองสุดท้าย

Category/Quartile ใน presentation รวมช่วงปี; KKU/Thailand, COC/Thailand, COC/KKU ใช้จำนวนรวมเป็นตัวตั้ง/ตัวหาร T1 และ non-Journal แยกกลุ่มตาม metric rules เดิม คง missing/partial year states ไม่แสดงปี missing เป็น 0

ส่งออก `view=presentation&report_view=presentation&revision=...` สร้างชีตผลกระทบการกรอง รายปี Category รวมช่วงปี Quartile รวมช่วงปี และคำอธิบาย ค่าเปอร์เซ็นต์เป็น numeric fractions ใช้ percent format รวม presentation aggregates ใน revision เพื่อให้ผลงานที่เปลี่ยนในฐานก่อนกรอง (แม้ไม่ผ่าน filters สุดท้าย) ทำให้ export ต้อง refresh

ข้อแตกต่างจากชีต summary ของ Excel อ้างอิง: ทุกระดับใช้ฐาน Thailand เดียว, ประเภทตาม filters ไม่ใช่การตัด Book/Book Series/Conference แบบตายตัว, ใช้ metric จาก DB ไม่อ่านเลข cache/formula จาก workbook เดิม

เปิดหน้าเรียก options/summary เท่านั้น; faculty เมื่อเปิด subview; details เมื่อกดจำนวน แท็บวิเคราะห์ mount เมื่อเคยเปิด หยุด comparison/insights เมื่อ inactive ใช้ cache เมื่อกลับ Setup เริ่ม scopes/runs/comparison เมื่อเปิด setup; งานที่เริ่มไว้ polling runs เบาๆ เพื่อแจ้ง stale หลังจบ

Setup writes/harvest ทำเครื่องหมาย cache เก่า แสดง refresh ไม่ reload รายงานทุกแท็บ มี AbortController และ generation กัน response เก่าทับ filters ใหม่

แท็บวิเคราะห์ยังใช้ scopes/snapshots/ความพร้อมและ employment refinement เดิม ยอดอาจต่างจากรายงานใหม่ฐาน Thailand/AF-ID 5/2/ไม่กรองวันเริ่มงาน ห้ามเติม core ที่อยู่นอกฐานเพื่อทำยอดให้เท่ารายงานเก่า

## ผลตรวจ dev 2026-09-30 ก่อน harvest และนำเข้า Excel

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

## Harvest แบบชุดและการนำเข้า Category จาก Excel

คำสั่ง harvest บันทึก search page ทั้งหน้า (25 ผลงาน) ภายใน transaction เดียว โดย batch catalogue ผู้เขียน/affiliation และความสัมพันธ์ เพื่อเลี่ยง round trip ทีละผู้เขียน ไม่ใส่ classification columns ใน INSERT/UPDATE จึงรักษา Category/confidence/model/taxonomy/classified_at แม้มีการนำเข้า Excel พร้อมกัน เลือก ID จริงกลับด้วย EID/Author ID/AF-ID หลัง upsert เพราะ auto-increment ของ mixed insert/update batch ใช้จับคู่ไม่ได้ ตรวจ author-count ก่อนตัดผู้เขียนเก่า และคงหลักฐาน affiliation ที่ payload ยังตรวจไม่ครบ CLI pin physical connection สำหรับ named lock

คำสั่งสำหรับ dev (years-back 2 ณ ปี 2026 คือ 2025–2026):

```powershell
go run ./cmd/scopus-benchmark -scope country_thailand -years-back 2 -expect-database <DEV_DATABASE> -quiet-sql
# Counts ใช้ scope ที่ระบุเท่านั้น: all-years และสองปีล่าสุด รวม 3 requests
go run ./cmd/scopus-benchmark -counts-only -scope country_thailand -years-back 2 -expect-database <DEV_DATABASE> -quiet-sql
# ขอหยุดรอบที่ระบุอย่างมีสถานะ รอจบ page ปัจจุบันก่อนเริ่มรอบใหม่
go run ./cmd/scopus-benchmark -cancel-run <RUN_ID> -expect-database <DEV_DATABASE> -quiet-sql
```

`cmd/scopus-benchmark-import-classification` อ่านไฟล์โดยไม่แก้ workbook:

- ใช้ `classified_scopus-benchmark-doc` เป็น classification ของฐาน Thailand; `combined_thailand-2025-2026` ตรวจแล้วตรงกันทั้งหมด ไม่เอาค่า confidence ของชีต COC มาทับฐาน Thailand (หลังเทียบภาษาไทย/อังกฤษ confidence ต่างกัน 12 รายการ แต่ Category ไม่ต่าง)
- ใช้ `raw_data` จับคู่ Scopus ID → EID ที่ระบุจริง แล้วจับคู่ benchmark ด้วย EID ตรวจ duplicate/ambiguous/missing keys และชื่อ Category ที่ไม่รู้จักก่อนเขียน ไม่ยึด category_id ของ production
- Excel มี 8,853 ผลงาน: 6,360 มี Category + High/Medium/Low; 2,493 ไม่มี Category และเป็น Needs Review จึงไม่แปลงเป็น Medium/Low/Preface และคงไม่จัดหมวด
- เติมเฉพาะผลงานปัจจุบันปี 2025–2026 ซึ่ง Category และ confidence ยัง NULL ทั้งคู่ ไม่สร้างเอกสารจาก Excel ไม่เปลี่ยนปี affiliation metrics หรือ core XML roles ไม่เขียนทับ classification ที่ขัดกัน
- ปีใน Excel ต่างจาก Scopus ปัจจุบันแต่ยังอยู่ในช่วง 2025–2026 ใช้ Category ของ EID เดิมได้ โดยเก็บปีปัจจุบันของ DB และบันทึก EID ที่ปีต่างใน audit ถ้าปัจจุบันอยู่นอกช่วงไม่เติม
- provenance: `classification_model=excel_import`, `classification_taxonomy_version=excel_final_2025_2026`, `classified_at=เวลา import UTC`; เป็นชื่อแหล่งนำเข้า ไม่ใช่การอ้างว่าได้ยิง AI model ใหม่ หรือรู้วัน/model ของการจัดหมวดครั้งดั้งเดิม audit JSON ระบุชื่อไฟล์ SHA-256 และ EID ทุกแถวที่เติม/ไม่พบ/ขัดกัน
- ใช้ atomic batch UPDATE และตรวจ NULL ซ้ำใน SQL ป้องกัน concurrent classifier ถูกทับ รันซ้ำจะข้ามค่าเดียวกัน ตรวจ fingerprint core XML roles ก่อนและหลัง import

```powershell
# Dry run ก่อน: ไม่มี -apply จะไม่เขียนข้อมูล
go run ./cmd/scopus-benchmark-import-classification -file ../classified_scopus-benchmark-documents-thailand-2025-2026_FINAL.xlsx -expect-database <DEV_DATABASE> -report ../tmp/scopus-excel-import-dry-run.json
# เติมช่องว่าง (รันระหว่าง harvest แบบชุดได้ และรันอีกครั้งหลัง harvest จบ)
go run ./cmd/scopus-benchmark-import-classification -file ../classified_scopus-benchmark-documents-thailand-2025-2026_FINAL.xlsx -expect-database <DEV_DATABASE> -apply -report ../tmp/scopus-excel-import-final.json
# Dry run ซ้ำ: pending=0 และ conflicts=0 สำหรับข้อมูลที่เติมสำเร็จ
go run ./cmd/scopus-benchmark-summary -audit -year-from 2025 -year-to 2026
```

ไม่ทำให้ยอด dev เท่ากับ Excel โดยเพิ่มผลงานที่ไม่ได้อยู่ในผล harvest ปัจจุบัน หลังเติมข้อมูล ให้กดอัปเดตรายงานบน UI เพราะผลที่โหลดไว้ก่อน import ยังเป็น cache เดิม

### การกู้รอบที่หยุดก่อน finalize

ระหว่างตรวจ dev พบว่า GORM handle จาก `Connection` เป็น initialized statement: subquery ที่ใช้ reconcile EID ปลายปีแรกอาจค้างบน handle แล้วปนกับ query ของปีถัดไป แก้ให้ `NewScopusBenchmarkService` ใช้ `Session(NewDB:true)` ซึ่งยังเก็บ physical connection/context แต่ทุก builder เริ่ม statement ใหม่ เพิ่ม regression test สำหรับเงื่อนไข EID ไม่ปนกับ run finalization และ rerun ทั้งช่วงหลังแก้

หาก process หยุดไปแล้วจริงแต่ run ค้าง running ให้ตรวจว่าไม่มี process/harvest lock ของรอบนั้น ก่อนใช้:

```powershell
go run ./cmd/scopus-benchmark -recover-failed-run <RUN_ID> -expect-database <DEV_DATABASE> -quiet-sql
```

คำสั่งนี้เปลี่ยน orphaned running เป็น failed เพื่อให้ประวัติสะท้อนปัญหาจริง ไม่เปลี่ยนผลงานที่ commit แล้ว จากนั้น harvest ซ้ำได้อย่างปลอดภัย ห้ามใช้กับ process ที่ยังทำงาน

## ผล harvest และนำเข้า Excel ใน dev 2026-09-30

- Target ตรวจด้วย `SELECT DATABASE()` ว่าเป็น `drnadech_fund_cpkku_intern` ทุกครั้ง ไม่แก้ production
- Scope `country_thailand`: 2025 = **5,628**, 2026 = **3,668**, รวม EID ไม่ซ้ำ **9,296**; run **18 success**, 373 pages / 375 successful Search requests; เวลา 06:00:29–06:14:24 +07:00
- Refresh count snapshot ของ Thailand เพิ่ม 3 requests (all-years/2026/2025): 55,236 / 3,668 / 5,628 รายงานทั้งสองปีเป็น available และ observed = expected
- นำเข้า Category/confidence จาก source ได้ **6,321** รายการ รันซ้ำ `already_same=6321`, `pending=0`, `conflicts=0`, `updated=0`; core XML roles fingerprint ก่อน/หลังไม่เปลี่ยน
- Source 6,360 classified: จับคู่ไม่ได้ **39** EID ซึ่งทั้งหมดพบในชีต COC ของ source แต่ไม่อยู่ใน benchmark dev หลัง harvest ครบ ไม่เพิ่มเป็นฐาน Thailand เอง ยังไม่ได้สรุปสาเหตุว่าเกิดจาก scope/ปี/subject area/metadata; เก็บชื่อและรายละเอียดให้ตรวจต่อ
- 38 ใน 39 รายการผ่าน Journal + มี Category + non-Low จึงอธิบาย Thailand default 1,873 ใน Excel → **1,835** ในรายงาน dev หลังใช้ฐานล่าสุด ไม่ปรับตัวเลขให้เท่า source
- ปีใน source ต่างจากปี DB ปัจจุบัน **2** รายการ: `2-s2.0-105024787304`, `2-s2.0-105008465302`; ทั้งคู่ยังอยู่ในช่วง 2025–2026 จึงเติม Category ตาม EID และรักษาปีของ DB
- Source **2,493** Needs Review/ไม่มี Category คงไม่จัดหมวดตามเดิม ไม่ยิง AI ใหม่ ไม่คัดลอก Quartile/percentile จาก Excel

Default report (Journal, classified, non-Low, แยก T1):

| ปี | Thailand | KKU | COC |
| --- | ---: | ---: | ---: |
| 2025 | 1,046 | 85 | 24 |
| 2026 | 789 | 70 | 24 |
| รวม | 1,835 | 155 | 48 |

Coverage ของ default report: selected 1,835; document affiliation incomplete 1, faculty-author affiliation incomplete 0, unknown role 2 คู่, metric fallback 910, missing Quartile 350, faculty without Scopus ID 4 ภาพรวมบทบาท COC: First 4, Corresponding 26, Lead 27, co-only 19, unknown 2 (First/Corresponding นับซ้อนกันได้)

ประวัติการลองรัน: run 16 บันทึก 200 รายการ/8 requests ก่อนขอหยุดเพื่อเปลี่ยนเป็น batch; run 17 บันทึกปี 2026 ครบ 3,668 แต่ process หยุดจากปัญหา initialized GORM statement ที่ปลายปี และถูกกู้เป็น failed; run 18 หลังแก้สำเร็จครบทั้งช่วง จำนวน request ที่บันทึกใน run 17 ไม่รวม request ของ page ที่ rollback/หยุดก่อน persist progress จึงไม่ใช่ยอด physical HTTP attempts ทั้งหมด

Regression สำหรับ statement ไม่ปนกัน, workbook EID/duplicate/ambiguous/unsupported confidence และ `go test ./...` ผ่าน รวมทั้งตรวจ report ผ่าน service จริงบน DB dev ผลตรวจรันซ้ำครอบคลุม classification ที่ถูก harvest ซ้ำและ import ระหว่าง harvest

ผลตรวจแบบอ่านได้ด้วยโปรแกรม พร้อม SHA-256 source, run, filters, coverage และรายละเอียด 39 EID: [scopus-benchmark-dev-2025-2026-import-audit.json](scopus-benchmark-dev-2025-2026-import-audit.json) (ไม่มี credential หรือ API key) รายงานนี้เป็น snapshot ของ dev ณ เวลาตรวจ ไม่ใช่สถานะ production


### ปรับ Confidence filter ในรายงาน

UI และ GET options ไม่เสนอ Preface และ default parser ใช้ High/Medium/unknown ตรงกับ frontend เพราะ Preface เป็นสถานะจัดหมวดหมู่ไม่ได้ ไม่ใช่ระดับความมั่นใจ เก็บ ENUM และ validation ของคำขอ Preface เดิมไว้เพื่อรองรับ clients เก่า ไม่เปลี่ยนข้อมูลที่บันทึกหรือประวัติ audit/ผล Excel ที่เคยนำเข้า

### Drilldown identity / EID mapping (2026-09-30)

`SummaryDocument.EID` ต้องมี `gorm:"column:eid"`: naming strategy ของ GORM ตี EID เป็น e_id ทำให้ SELECT d.eid ไม่เติมฟิลด์ แม้แถว DB มี EID ส่งผลให้ JSON eid ว่างและ React key ซ้ำ ข้อแก้เป็น read mapping เท่านั้น ไม่แก้ EID ใน DB

Documents response เพิ่ม id (benchmark document PK) และ authors[].author_id (benchmark author PK) สำหรับ identity ของแถว UI โดย Scopus EID/Author ID ยังคงใช้เชื่อมผลงาน/บทบาทตามเดิม Regression test ตรวจ column lookup/set และ JSON identity รวมผู้เขียนที่ไม่มี Scopus ID

### ค้นหาและกรองรายการ drilldown (2026-09-30)

`GET /summary/documents` รองรับ query เพิ่มเติม:

| Query | ความหมาย |
| --- | --- |
| `search` | ไม่เกิน 200 Unicode characters ค้นหา title, EID, DOI, แหล่งตีพิมพ์, ชื่อผู้เขียน และ Scopus Author ID แบบไม่แยกตัวพิมพ์เล็กใหญ่; คำที่คั่นด้วยช่องว่างต้องพบครบทุกคำ |
| `filter_category` | Category ID สำหรับกรองย่อย; `0` คือไม่มี Category, ค่าว่างไม่กรองเพิ่ม |
| `filter_quartile` | `T1`, `Q1`–`Q4`, `missing`, `not_applicable`; ค่าว่างไม่กรองเพิ่ม |

ใช้ filter กลางของรายงาน → context ของเซลล์ที่คลิก (`level`, `year`, `document_category`, `quartile`, `user_id`, `role`) → ตัวกรองย่อยและคำค้น → นับ total และแบ่งหน้า 50 รายการ ตัวกรองย่อยไม่ขยาย cohort เดิม Response เพิ่ม `document_filters` เพื่อระบุค่าที่ใช้ และรักษารูปแบบเดิมเมื่อไม่ส่ง query ใหม่ ไม่มีการเรียก Scopus หรือแก้ DB ในเส้นทางนี้

`FilterSummaryDocumentList` มี regression tests ตรวจผลค้นหาที่อยู่นอก 50 รายการแรก, ภาษาไทย/Author ID/EID/DOI, ตัวกรอง Category + Quartile, ไม่มี Category และการรักษาขอบเขต cohort เดิม ใช้ผลรายงานเดิมเป็นฐานและเก็บ revision/coverage เดิม
### ผลตรวจ presentation report บน dev (2026-09-30)

`go test ./services ./controllers -count=1` ผ่าน รวม fixtures ขั้นการกรองที่สะสม, EID ไม่ซ้ำและขอบเขตปี, nested cohort, stage filters ที่เปิดรายการได้ตรงยอด, missing year/ศูนย์จริง, metric buckets และ T1 toggle, revision ที่เปลี่ยนเมื่อผลงานก่อนกรองเปลี่ยน และ Excel readback ที่เก็บ counts/ratios เป็นตัวเลข

บน DB dev ปี 2025–2026 ได้ baseline Thailand 9,296 / KKU 559 / COC 126; หลังประเภท Journal 5,333 / 400 / 74; หลังมี Category 3,362 / 250 / 62; หลัง Confidence High/Medium/unknown 1,835 / 155 / 48 ตัวเลขสุดท้ายตรงกับรายงานปกติ รายปี Category และ Quartile และรักษา COC ≤ KKU ≤ Thailand สัดส่วนคงเหลือใช้ baseline ของระดับเดียวกัน ตัวอย่าง Thailand หลังกรอง 19.7% ไม่ใช่สัดส่วนเทียบขั้น Category

Browser ตรวจ stage drilldown ของ COC baseline ได้ 126 รายการและ export พร้อม revision ได้ HTTP 200; Blob download event ไม่ถูกส่งกลับจาก in-app browser จึงตรวจความถูกต้องของไฟล์ด้วย service XLSX readback tests ชุดนี้ ไม่มี Scopus request, migration หรือ DB write ในงานแท็บใหม่นี้ ผล dev ไม่ใช่ตัวเลข production หรือยอดใน workbook เก่า
