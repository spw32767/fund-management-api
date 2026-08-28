# งาน: เพิ่มการ์ด h-index "ระดับคณะ" (faculty-level) ต่อท้ายการ์ดรายบุคคลในหน้า dashboard

## ก่อนเริ่ม (บังคับ)
1. อ่าน `fund-management-api/docs/SCOPUS_AUTHOR_HINDEX.md` ให้จบก่อน = สรุปสถานะฟีเจอร์ h-index ปัจจุบันทั้ง backend+frontend
2. **ใช้ branch เดิม `feature/scopus-author-hindex`** (งานนี้ต่อยอดฟีเจอร์ h-index เดิม เก็บงานเกี่ยวเนื่องไว้ที่เดียว) — checkout branch นั้นทั้ง 2 repo แล้ว **sync กับ main ก่อนเริ่ม**: `git fetch origin && git merge origin/main`
   - frontend: branch ตามหลัง main 1 commit (chunk-error fix) → ต้อง merge เข้ามาก่อน ไม่งั้นจะขาด fix
   - backend: branch ตรงกับ main อยู่แล้ว (ไม่มีอะไรต้อง merge)
   - อย่าเพิ่ง merge branch เข้า main จนกว่าจะได้รับอนุญาต
3. DB ที่ `.env` ชี้ (147.50.227.17 / drnadech_fund_cpkku_intern) เป็น **TEST DB** — prod เป็น XAMPP local บน VM คนละตัว อย่าสับสน
4. commit ขึ้น branch อย่าเพิ่ง merge main จนกว่าจะได้รับอนุญาต · เขียน commit message ด้วย git syntax ที่ถูก (อย่าใช้ here-string `@'...'@` ของ PowerShell กับ Bash tool — มันจะแปะ `@` หน้า message)

## ขอบเขต
เพิ่ม **การ์ดใหม่ระดับคณะ ต่อท้าย(ด้านล่าง)การ์ดรายบุคคลเดิม** ในหน้า `/research-dashboard`
ของเดิม (การ์ดรายบุคคล + endpoints + ปุ่มต่าง ๆ) **คงไว้ทั้งหมดไม่ต้องแตะ** — แค่เพิ่มของใหม่เข้าไป

## สิ่งที่ต้องทำ

### A) Backend — คำนวณ h-index ระดับคณะ
คำนวณจาก `scopus_documents` โดย:
- รวมเอกสารของ **อาจารย์ทุกคนที่มี `users.scopus_id`** (ไม่ deleted)
- **กรองเฉพาะเอกสารที่ผู้เขียนคนนั้นสังกัด KKU ตอนตีพิมพ์** — join `scopus_document_authors sda` → `scopus_affiliations aff` แล้วกรอง `LOWER(aff.name) IN (kkuAffiliationNames)` **reuse constant `kkuAffiliationNames` จาก `services/scopus_publication_service.go`** (อย่า hardcode ใหม่ ให้ใช้ตัวเดียวกับหน้า research-search เพื่อให้ตรงกัน)
- **dedupe ต่อ document** (paper ที่อาจารย์ KKU ร่วมกัน ≥2 คน ต้องนับครั้งเดียว) — GROUP BY document id
- จากชุดเอกสารที่ deduped: เรียง citedby_count มาก→น้อย, คำนวณ h-index (Hirsch มาตรฐาน: h = ค่า n มากสุดที่ doc อันดับ n มี citation ≥ n) — logic เดียวกับ `AuthorHGraphService.GetGraph`
- รองรับ filter ช่วงปี (year_from/year_to, ค.ศ.) แบบเดียวกับ GetGraph
- คืน shape เดียวกับ `AuthorHIndexGraph` (h_index, document_count, citation_total, available_years, points[{rank,citations,title,year,eid}]) เพื่อให้ frontend reuse การวาดกราฟได้

**ไฟล์ที่เกี่ยวข้อง:**
- เพิ่ม method เช่น `GetFacultyGraph(ctx, yearFrom, yearTo)` ใน `services/scopus_author_hgraph_service.go`
- เพิ่ม controller + route: `GET /api/v1/admin/scopus/author-metrics/faculty-hgraph` (ดูรูปแบบจาก `AdminGetScopusAuthorHIndexGraph` ใน `controllers/admin_scopus_author_metrics_controller.go` + `routes/routes.go` บล็อก scopus author-metrics)
- **สำคัญ — ต่างจากรายคน:** h-index รายคน (ของเดิม) ใช้ผลงานทั้งหมดทั่วโลกเพื่อ match Scopus แต่ระดับคณะ **ใช้เฉพาะผลงานที่สังกัด KKU** → เป็นคนละสูตร อย่าเอา per-author summary มา union ตรง ๆ ต้องคิวรีใหม่พร้อม KKU filter + dedupe
- endpoints เดิมคงไว้ ไม่ต้องแก้

### B) Frontend — การ์ดระดับคณะ (เพิ่มต่อท้าย)
- สร้าง component ใหม่ `AdminScopusFacultyHIndex.js` (mirror `AdminScopusAuthorHIndex.js`) แต่:
  - **ไม่มี dropdown เลือกอาจารย์** (เป็นทั้งคณะ)
  - คง: ตัวกรองช่วงปี (พ.ศ.), กราฟ Hirsch + zoom/pan (ยกโค้ด zoom/pan มาใช้ได้เลย), ปุ่ม export CSV รายการบทความของคณะ, รายงาน HTML พร้อมกราฟ
  - หัวข้อ/label: "h-index ระดับคณะ (Scopus)" + หมายเหตุว่านับเฉพาะผลงานสังกัด KKU
  - เรียก api ใหม่ `getFacultyHIndexGraph({year_from, year_to})`
- `app/lib/api.js` (`scopusConfigAPI`): เพิ่ม `getFacultyHIndexGraph`
- ในหน้า `AdminScopusResearchDashboard.js`: **เพิ่ม `<AdminScopusFacultyHIndex />` ต่อท้ายด้านล่างของ `<AdminScopusAuthorHIndex />` ที่มีอยู่** (การ์ดรายบุคคลเดิมคงไว้ตามเดิม)

## จุดที่ควรถาม user ก่อนทำ (อย่าเดา)
- ต้องการให้การ์ดคณะแสดงค่าเดียว ณ ปัจจุบัน หรือมีเทียบช่วงปี/trend เพิ่มด้วย

## Verify
- backend: `go build ./...` + `go test ./services/`
- frontend: `npm run build` + (ถ้า login ได้) เปิดหน้า research-dashboard เลื่อนลงล่างสุดดูการ์ดคณะใต้การ์ดรายบุคคล + ทดสอบ export/zoom
- ตรวจว่า h-index คณะ **สมเหตุผล** (ควร ≥ h-index ของอาจารย์เก่งสุด และ ≤ ผลรวม เพราะเป็น union ที่ dedupe แล้ว)

## หมายเหตุความถูกต้อง
- ค่า h-index ขึ้นกับความสด `citedby_count` (ค่า ณ ตอน ingest) — ต่ำกว่า scopus.com ได้ถ้า citation เก่า
- KKU filter ตอนนี้เป็น "by name" (อนาคตอาจย้ายไป AFID) — ใช้ constant กลางจะ migrate ง่าย
