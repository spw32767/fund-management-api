# Database Migrations

ไฟล์ในโฟลเดอร์นี้ตั้งชื่อด้วยเลขลำดับ `NNN_YYYYMMDD_<description>.sql`
**ให้รันเรียงตามเลขลำดับ (`001`, `002`, ...) เท่านั้น** อย่ารันเรียงตามวันที่
เพราะบางไฟล์วันเดียวกันมี dependency ต่อกัน (เช่น ต้อง `CREATE TABLE` ก่อน `ALTER TABLE`)

## วิธีรัน (apply ทีละไฟล์ตามลำดับ)

```bash
for f in $(ls -1 [0-9]*.sql | sort); do   # จับทั้ง 012_ และไฟล์แทรกอย่าง 012a_
  echo ">> $f"
  mysql -u <user> -p<password> <database> < "$f"
done
```

> หมายเหตุ: ไฟล์ที่แทรกระหว่างลำดับจะใช้ suffix ตัวอักษร เช่น `012a_` ซึ่ง `sort`
> จะเรียงให้อยู่**หลัง** `012_` และ**ก่อน** `013_` โดยอัตโนมัติ

## ลำดับการรัน

| ลำดับ | ไฟล์ | หมายเหตุ |
|------|------|----------|
| 001 | 20260309_add_sso_auth_identities | |
| 002 | 20260316_support_fundmapping_status_enum_and_rename_columns | |
| 003 | 20260321_add_permission_system | |
| 004 | 20260322_add_access_control_ui_permissions | |
| 005 | 20260323_reset_role_permission_baseline | |
| 006 | 20260324_permission_catalog_phase2 | |
| 007 | 20260325_add_access_view_permission | |
| 008 | 20260421_add_admin_research_dashboard_permission | |
| 009 | 20260426_add_thaijo_integration_tables | **CREATE ตาราง thaijo_* — ต้องรันก่อน 010/011** |
| 010 | 20260426_add_thaijo_author_selection_reason | ALTER thaijo_api_import_jobs (ตารางมาจาก 009) |
| 011 | 20260426_add_thaijo_document_abstracts | ALTER thaijo_documents (ตารางมาจาก 009) |
| 012 | 20260501_create_ai_showcase_tables | ต้องรันก่อน view 013/014/023 |
| 012a | 20260706_add_role_to_ai_showcase_project_members | เพิ่มคอลัมน์ `role` — **ต้องรันก่อน 023** เพราะ view `unified_search_authors` เลือก `m.role` (แทรกไว้ที่นี่แทนต่อท้าย ไม่งั้น 023 จะ error `Unknown column 'm.role'`) |
| 013 | 20260501_create_unified_search_authors_view | ดู "ปัญหาที่ยังค้าง" ข้อ 2 |
| 014 | 20260501_create_unified_search_contents_view | |
| 015 | 20260502_create_education_and_course_tables | สร้าง instructor_degrees/courses/educations — ต้องรันก่อน 020 |
| 016 | 20260502_create_instructor_works_tables | |
| 017 | 20260502_create_instructor_edit_logs_table | (เดิมชื่อไฟล์มีเว้นวรรค แก้แล้ว) |
| 018 | 20260502_create_tier_weights_table | สร้าง ranking_sources / ranking_tier_weights |
| 019 | 20260507_update_ai_showcase_poster | |
| 020 | 20260510_create_instructor_course_responsibility | FK -> instructor_courses (จาก 015) |
| 021 | 20260522_create_instructor_intellectual_properties | |
| 022 | 20260609_add_id_to_instructor_course_responsibility | **ทั้งไฟล์ถูก comment ไว้ = ไม่ทำอะไร** (ดูข้อ 3) |
| 023 | 20260609_recreate_unified_search_views | สร้าง unified_search_contents + unified_search_authors ใหม่ (เวอร์ชันจริงที่ใช้) |
| 024 | 20260613_create_mou_tables | **ต้องมี `countries` และ `faculties` ก่อน** (ดูข้อ 1) |
| 025 | 20260619_insert_new_role_into_roles | |

## ⚠️ สิ่งที่ยังขาด — ต้องขอจาก DB intern ก่อนใช้งานจริง

migration set นี้ยัง **สร้าง schema ของ intern ได้ไม่ครบ** ถ้าเอาไปรันบน DB เปล่าจะ error/ตารางไม่เท่ากัน
รายการต่อไปนี้มีอยู่ใน DB intern แต่ **ไม่มีไฟล์ migration** (น้องฝึกงานสร้างมือ) — ต้องขอไฟล์เพิ่ม:

1. **ตาราง `countries`** และ **`faculties`** — ถูกอ้างเป็น FK ใน `024_..._create_mou_tables.sql`
   ถ้าไม่มีตารางสองตัวนี้ก่อน ไฟล์ 024 จะรันไม่ผ่าน
2. **คอลัมน์ conference ใน `scopus_documents`** (7 คอลัมน์):
   `conference_name, conference_venue, conference_city, conference_country,
   conference_location, conference_info_json, conference_info_fetched_at`
3. **คอลัมน์ `course_id` ใน `users`** (`int(11) NULL`)
4. **ข้อมูล seed** ของตาราง lookup (countries, faculties, mou_status, mou_partner_type,
   mou_activity_type, ranking_sources, ranking_tier_weights, ai_showcase_tracks)
   — dropdown / FK จะใช้งานไม่ได้ถ้าไม่มีข้อมูลตั้งต้น

## ปัญหาที่ยังค้าง (ควรแก้ตอนน้องส่งไฟล์ที่ขาดมา)

1. ดูข้อ "สิ่งที่ยังขาด" ข้อ 1 ด้านบน — ต้องเพิ่ม migration สร้าง `countries` / `faculties`
   ให้มีลำดับ **ก่อน** 024
2. `013_..._authors_view.sql` จริงๆ ข้างในเขียน `CREATE VIEW unified_search_contents`
   (ชื่อ view ผิด ควรเป็น `unified_search_authors`) และใช้ `CREATE VIEW` เฉยๆ ไม่ใช่
   `CREATE OR REPLACE` — รันซ้ำจะ error ตอนนี้ผลลัพธ์สุดท้ายถูก 023 recreate ทับให้อยู่แล้ว
   แต่ควรแก้ให้ถูกเพื่อความสะอาด
3. `022_..._add_id_...sql` ถูก comment ทั้งไฟล์ แต่ตารางจริงใน intern มีคอลัมน์ `id` แล้ว
   (ไฟล์ 020 เวอร์ชันปัจจุบันสร้าง `id` มาให้ตั้งแต่ต้น) — ไฟล์นี้จึงเป็น no-op เก็บไว้อ้างอิงได้
