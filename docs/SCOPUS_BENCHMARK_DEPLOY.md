# Deploy: Scopus Benchmark summary

ตรวจ release วันที่ 4 ตุลาคม 2026 จาก branch `feat/scopus-benchmark-summary` ทั้งสอง repo

## ผลตรวจและขอบเขต

- Remote main ไม่มี commit ที่ branch นี้ขาด ทั้งสอง repo merge แบบ fast-forward ได้
- `go test ./...` ผ่าน รวม benchmark insights/export เดิม, ingest, author roles, routes/controllers และ summary tests ที่มีใน repo
- Frontend helper tests รวม PublicationRewardForm ผ่าน 71 กรณี; production `next build` ผ่าน compile, page generation และ build traces
- ไม่มี dependencies หรือ environment variables ใหม่สำหรับ release นี้ ไม่ต้อง install package เพิ่มเพราะ feature นี้ หาก server มี dependencies ของ main เดิมครบ
- UI ที่แก้คือหน้า `/research-fund-system/admin/scopus-benchmark` รวมรายงานใหม่, lazy loading และ default ปีของแท็บวิเคราะห์ Hint ที่เปลี่ยนใช้ใน benchmark เท่านั้น ไม่ใช่ tooltip ทั้งเว็บไซต์
- `APIClient.get` เพิ่ม options ตัวที่สาม เช่น AbortSignal; default เป็น `{}` ผู้เรียก GET เดิมหนึ่ง/สอง argument ยังใช้ได้
- API รายงานใหม่เป็น GET แยกจาก endpoint เดิม ใช้ read-only snapshot ไม่ยิง Scopus/AI หรือเขียน DB เมื่อเปิดรายงาน
- Benchmark ingest เปลี่ยนเป็น batch upsert และเก็บหลาย affiliation จึงต้องลง 049 ก่อนเปิด backend ใหม่ ไม่เขียน core XML roles หรือเปลี่ยน import ของ ThaiJO/Google Scholar/KKU Profile
- Component ใหม่เพิ่ม code ใน bundle ที่หน้า admin import อยู่แล้ว Build ผ่าน แต่ไม่ได้วัด production performance หรือทดสอบทุก flow ด้วย production accounts จึงไม่รับรองว่าไม่มีผลทุกหน้าในทุกกรณี
- ไม่มีการเข้าถึงหรือแก้ production DB ในการตรวจนี้ ต้อง smoke test บน server จริงหลัง deploy

## ก่อน deploy

1. สำรอง production DB และเก็บ binary/backend กับ frontend build รุ่นปัจจุบันสำหรับ rollback จด commit ที่ server ใช้ก่อนเปลี่ยน
2. เลือกช่วงไม่มี benchmark harvest/backfill กำลังเขียน และพัก scheduled jobs ที่อาจชนระหว่าง migration/backfill
3. ตรวจ `.env`/environment ของ production โดยเฉพาะ `DB_DATABASE` รักษาค่าเดิม ไม่คัดลอก `.env` หรือ DB dev มาทับ
4. Checkout ทั้งสอง repo บน server ต้องเป็น `main` และ upstream `origin/main` **deploy scripts ทำ fetch/reset ตาม upstream ของ branch ปัจจุบัน ไม่สลับไป main ให้** ตรวจ tracked changes ก่อน เพราะ scripts จะทิ้ง tracked changes ด้วย reset --hard
5. อัปเดต source backend main เพื่อให้มี CLI/migration ใหม่ก่อนเปิด binary ใหม่ ลงเฉพาะ migration ที่ยังขาด ไม่รันทั้งโฟลเดอร์ซ้ำ

| Migration | ต้องทำอะไร |
| --- | --- |
| 047_20260929_add_scopus_author_roles.sql | เฉพาะยังไม่มี author_role_status/author_role_checked_at และ first/corresponding flags; **ไม่ rerunnable** ถ้าลงแล้วห้ามรันซ้ำ |
| 048_20260929_create_scopus_author_role_runs.sql | เฉพาะยังไม่มี scopus_author_role_runs สำหรับหน้าจัดการ XML; อยู่ใน main ก่อน release นี้แล้ว |
| **049_20260930_scopus_benchmark_summary.sql** | **migration ใหม่ของ release นี้ ต้องลงก่อน backend ใหม่** เพิ่ม classification fields ที่ขาด, affiliations_complete, ตาราง document/author affiliations และ taxonomy ที่ขาด รันซ้ำได้และรักษาค่าจัดหมวดเดิม |

ตรวจ schema ผ่าน DB client บน production ที่ตั้งใจ deploy:

```sql
SELECT DATABASE();
SELECT table_name, column_name FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND ((table_name = 'scopus_documents' AND column_name IN ('author_role_status','author_role_checked_at'))
    OR (table_name = 'scopus_document_authors' AND column_name IN ('is_first_author','is_corresponding_author'))
    OR (table_name = 'scopus_benchmark_documents' AND column_name IN ('category','classification_confidence',
         'classification_model','classification_taxonomy_version','classified_at','affiliations_complete'))
    OR (table_name = 'scopus_benchmark_document_authors' AND column_name = 'affiliations_complete'))
ORDER BY table_name, column_name;
SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name IN ('paper_categories','scopus_author_role_runs',
 'scopus_benchmark_document_affiliations','scopus_benchmark_author_affiliations');
```

Migration ไม่รันอัตโนมัติเมื่อ API เริ่ม และ deploy scripts ไม่รัน migration ให้ ตัวอย่าง PowerShell บนเครื่อง production จากโฟลเดอร์ backend:

```powershell
$releaseDatabase = 'ชื่อฐานข้อมูล production จริง'
go run ./cmd/scopus-benchmark-summary -migrate migrations/049_20260930_scopus_benchmark_summary.sql -expect-database $releaseDatabase
```

CLI โหลด `.env` และเขียนเมื่อ `-expect-database` ตรง `DB_DATABASE` ของ process เท่านั้น หาก service ใช้ environment แทน `.env` ต้องตั้ง environment ของ shell ให้ตรง service ก่อนรัน

## Deploy ตามลำดับ

1. ยืนยัน migration ที่ขาดสำเร็จ รวม 049 ก่อนใช้ binary ใหม่
2. Deploy backend main ผ่าน `deploy-backend.ps1` (build → stop/swap/start) รอ service Running และตรวจ log ว่าไม่มี unknown column/table
3. กู้ affiliation เก่าด้วย CLI ใหม่บนฐาน production เดิม แล้ว audit:

```powershell
go run ./cmd/scopus-benchmark-summary -backfill -expect-database $releaseDatabase
go run ./cmd/scopus-benchmark-summary -audit -year-from 2025 -year-to 2026
```

Backfill อ่าน JSON ที่มีอยู่ ไม่ใช้ Scopus request และไม่เปลี่ยน Category, XML roles หรือ membership หาก raw JSON ถูกล้างและกู้ไม่ได้ จะใช้ legacy first affiliation เป็นหลักฐานเชิงบวกและยังแจ้ง incomplete

4. Deploy frontend main ผ่าน `deploy-frontend.ps1` (build → restart) หลัง backend พร้อม Build ต้องติดต่อ Google Fonts เพื่อดึง Sarabun ตาม layout เดิม
5. เปิด jobs ที่พักไว้หลัง migration/backfill และ smoke test ผ่าน

## หลัง deploy

- Login และเปิดทั้ง 4 แท็บ benchmark ตรวจ lazy loading/cache และ filters แบบ draft/applied ค่าเริ่มต้นปีปัจจุบัน/ปีก่อนหน้า Journal มี Category High/Medium/ไม่ระบุ
- ยืนยัน `COC ≤ KKU ≤ Thailand`; ยอดหลังกรองในสองแท็บสรุปตรงกันเมื่อ filters เดียวกัน Category/Quartile รวมตรง total; T1/Q1–Q4 ไม่เปลี่ยน total
- กดยอดก่อน/หลังกรองได้รายการตรงจำนวน ค้นหา/แบ่งหน้า/ขยายรายละเอียดได้ ดูบทบาทรายอาจารย์และจำนวนที่ยังระบุไม่ได้
- ส่งออกและเปิด XLSX จริง ตรวจ counts/filters/โหมดตรงหน้าจอ เมื่อข้อมูลเปลี่ยนระหว่างอ่านกับ export ต้องแจ้งให้อัปเดตก่อน (409)
- ตรวจสถานะปีและ coverage ของ production: Thailand complete/partial, Category ว่าง, affiliation incomplete, unknown roles และ metric fallback/missing ไม่บังคับยอดเท่ากับ dev หรือ workbook เก่า
- Smoke test หน้าเดิมที่สำคัญ: `/admin/research-dashboard`, `/admin/academic-imports`, ค้นหาผลงาน/profile และคำขอทุน ตรวจ API/log ว่าไม่มี 500/SQL error
- เมื่อมี harvest ตามรอบปกติ ตรวจ classification เดิมยังอยู่และ affiliation ใหม่บันทึกได้ ไม่ต้องกดดึงใหม่ทั้งหมดเพื่อเปิดใช้รายงาน

## เติมข้อมูลเมื่อจำเป็น

- ชุด Thailand ขาดหรือ metadata กู้ไม่ได้: harvest country scope เฉพาะปีที่ต้องการหลัง deploy normalizer ใหม่ แล้วจัด Category ด้วย workflow เดิม ไม่ต้องเติม KKU/COC แยกเพื่อทำยอดให้เท่ากัน
- Category มีแล้วไม่ต้อง import workbook ซ้ำ หากจะเติมช่องว่าง ใช้ classification importer แบบ dry run ตรวจ audit ก่อน `-apply` ไม่คัดลอก DB dev ทับ production
- Unknown roles: หน้า `/admin/academic-imports` → Scopus → เติมข้อมูลที่ยังไม่ตรวจ ใช้ XML หนึ่ง request ต่อผลงานที่ยังไม่ตรวจ ต้องมี key/เครือข่ายที่เข้าถึง Scopus ไม่เลือก refresh ทั้งหมดโดยไม่จำเป็น
- `-verify-migration` สร้าง/ล้าง fixture tables ไม่ใช่ read-only preflight และไม่จำเป็นบน production

## Rollback

คืน backend binary และ frontend build/source ของ commit ก่อน deploy ทั้งคู่แล้ว restart services คง additive schema 049 ไว้ ไม่ DROP ตาราง/columns หรือย้อนลบ classification/backfill เพื่อ rollback แอป ใช้ DB backup เฉพาะเมื่อมีปัญหาข้อมูลที่ยืนยันแล้ว

สูตร/API/backfill: [SCOPUS_BENCHMARK_SUMMARY.md](SCOPUS_BENCHMARK_SUMMARY.md)
