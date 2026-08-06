# Scopus Benchmark — Logic Audit

ตรวจสอบความถูกต้องของ logic ทั้งหมด (Scopus query, การนับ, SQL เปรียบเทียบ, การ upsert)
ผลสรุป: **logic หลักถูกต้องทั้งหมด** — ยืนยันด้วยข้อมูลจริงในตอนท้าย มีเพียงข้อสังเกตเล็กน้อย (ไม่กระทบการใช้งาน)

---

## 1. การสร้าง Scopus query

ทุก scope กรองด้วย `SUBJAREA(COMP)` และ (ถ้าระบุปี) `PUBYEAR = <y>`

| scope | query | ตรวจ |
|---|---|---|
| **university (KKU)** | `AF-ID(<af>) AND SUBJAREA(COMP) [AND PUBYEAR=y]` | ✅ |
| **country (Thailand)** | `AFFILCOUNTRY(Thailand) AND SUBJAREA(COMP) [AND PUBYEAR=y]` | ✅ |
| **faculty** | `AF-ID(<KKU>) AND SUBJAREA(COMP) [AND PUBYEAR=y] AND (AU-ID(a) OR AU-ID(b) ...)` | ✅ |

- faculty เพิ่มเงื่อนไข `AU-ID(...)` ของอาจารย์ในระบบ ทับบน query ของ KKU → **faculty เป็น subset ของ KKU เสมอ** (การันตี `faculty ≤ university`)
- `code=faculty_cs` (level `faculty`) — af_id อ่านจาก scope university, รายชื่อ AU-ID จาก `users.scopus_id` (normalize ตัด `SCOPUS_ID:` + dedup)
- 🔸 *ข้อสังเกต:* branch `custom` ใน `buildScopeQuery` ไม่ได้ใส่ `extra_query` ลงใน query จริง (latent bug) — **ยังไม่ถูกใช้** (มีแค่ university/country/faculty) จึงไม่กระทบ

## 2. การนับ + snapshot

- `CountScope` ยิง `count=1` อ่าน `opensearch:totalResults` = **จำนวนเอกสารไม่ซ้ำ** (Scopus dedup ให้)
  → เปเปอร์ที่มีอาจารย์ร่วมหลายคน = นับ **1** ครั้ง (ไม่ double-count) ✅
- ทุกครั้งที่นับจะเขียน snapshot row ใหม่ (เก็บประวัติ) — การเปรียบเทียบใช้ **row ล่าสุด**

## 3. SQL เปรียบเทียบ (`AdminGetBenchmarkComparison`)

```sql
-- ต่อ scope: เอา total ของ snapshot "ล่าสุด" ในแต่ละปี
SELECT s.pub_year, s.total_results
FROM scopus_benchmark_count_snapshots s
JOIN (SELECT pub_year, MAX(captured_at) mx
      FROM scopus_benchmark_count_snapshots
      WHERE scope_id = ? AND pub_year IS NOT NULL
      GROUP BY pub_year) latest
  ON latest.pub_year = s.pub_year AND latest.mx = s.captured_at
WHERE s.scope_id = ?
```
- คำนวณ 3 แผนที่ (`facultyByYear`, `uniByYear`, `countryByYear`) แล้วประกอบเป็นแถวรายปี ✅
- เก็บผลลง `map[year]total` → **ถึงมี snapshot ซ้ำ วินาทีเดียวกัน ก็ไม่บวกเกิน** (map เขียนทับด้วย key ปี)
- 🔸 *แนะนำเสริม:* ถ้าอยากกันกรณี `captured_at` ชนวินาทีเดียวกันเป๊ะ ใช้ `MAX(id)` เป็น tie-break แทน (ปัจจุบันตรวจแล้ว **ซ้ำ = 0**)

## 4. Year bounds (`benchmarkYearBounds`)

- ให้ความสำคัญ **range** (`year_from`/`year_to`) ก่อน; ถ้าไม่ส่งใช้ `years_back` (default 10)
- normalize: ให้ปีเดียว/ครึ่งช่วง/สลับ min-max ได้ถูกต้อง ✅

## 5. การ upsert ตอน harvest

- เอกสาร: unique ด้วย `eid` (find → create/save) ✅ idempotent
- ผู้เขียน: unique ด้วย `scopus_author_id` ✅
- membership: unique `(document_id, scope_id)` ✅
- `is_faculty`: authid (normalize) ตรงกับ set ของ `users.scopus_id` ✅
- `membership.pub_year` มาจาก `cover_date` ปีจริงของเอกสาร (ไม่ใช่ปีที่ query) → แม่นกว่า
  *(เอกสาร in-press บางชิ้น coverDate อาจต่างจากปีที่ query เล็กน้อย เช่น harvest 195 เก็บ pub_year=2026 ได้ 193 อีก 2 เป็นปีจริงอื่น)*

## 6. Pagination

| | วิธี | ลิมิต | ต้องมี VPN |
|---|---|---|---|
| counts | offset `start=0, count=1` | ไม่ต้อง paginate | ไม่ต้อง (STANDARD) |
| harvest | **cursor** (`*` → `@next`) | ไม่ตัน 5,000 | ต้อง (COMPLETE + cursor) |

- harvest หยุดเมื่อ cursor ไม่ขยับ (`@next` ว่างหรือเท่าเดิม) ✅

## 7. Invariant ที่ตรวจกับข้อมูลจริง

| ปี | คณะ | KKU | Thailand | `faculty ≤ KKU ≤ Thailand` |
|---|---|---|---|---|
| 2026 | 57 | 195 | 2,939 | ✅ |
| 2025 | 63 | 311 | 5,595 | ✅ |
| 2024 | 52 | 231 | 4,732 | ✅ |

- **Nesting เป็นจริงทุกปี** ✅
- **ไม่มี snapshot ล่าสุดซ้ำ** (double-count = 0) ✅
- **cross-check:** คณะจาก author-set count (57) = คณะจาก harvest `is_faculty` (57) → 2 วิธีอิสระให้ผลตรงกัน ✅

## 8. ข้อสังเกต / คำแนะนำ (ไม่กระทบการใช้งาน)

1. 🔸 `custom` scope query ไม่ประกอบ `extra_query` (latent, ยังไม่ใช้)
2. 🔸 comparison ใช้ `MAX(captured_at)` — พิจารณาเปลี่ยนเป็น `MAX(id)` กันชนวินาที
3. 🔸 snapshot row สะสมเรื่อยๆ — อาจเพิ่ม housekeeping ลบของเก่าในอนาคต
4. 🔸 faculty query ต่อ AU-ID ด้วย `OR` (ตอนนี้ 41 คน = 894 ตัวอักษร) — ถ้าอาจารย์เพิ่มเป็นหลักร้อย ควร chunk แล้ว dedup ด้วย eid (กันนับซ้ำข้าม chunk)
5. 🔸 ควรกด "อัปเดตตัวเลข" ทีเดียว (counts/refresh วนทุก scope พร้อมกัน) เพื่อให้ตัวเลข 3 ระดับมาจากช่วงเวลาเดียวกัน → nesting คงเส้นคงวา

**สรุป:** logic ถูกต้อง ปลอดภัยต่อการใช้งานจริง ข้อ 1–5 เป็นการปรับปรุงเผื่ออนาคต ไม่ใช่บั๊กที่กระทบตอนนี้
