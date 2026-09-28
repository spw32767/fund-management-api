# บทบาทผู้เขียนจาก Scopus Abstract Retrieval XML

## จุดประสงค์และขอบเขต

งานนี้เติมบทบาทผู้เขียนในฐานข้อมูล Scopus หลักเพื่อใช้กับผลงานของอาจารย์และต่อยอดรายงานหรือ profile ในอนาคต เลือก `scopus_documents` ที่มีผู้เขียนอย่างน้อยหนึ่งคนซึ่ง `scopus_authors.scopus_author_id` ตรงกับ `users.Scopus_id` โดย **ไม่กรอง affiliation หรือปีเริ่มงาน** จากนั้นเก็บบทบาทให้ผู้เขียน **ทุกคน** ของผลงานที่เลือก รวมถึงผู้เขียนนอกคณะ ผลงานหนึ่งชิ้นเรียก XML เพียงครั้งเดียวแม้มีอาจารย์หลายคน

งานนี้ไม่อ่านหรือเติมตาราง `scopus_benchmark_*` ของ KKU/Thailand และยังไม่เปลี่ยน dashboard หรือ API รายงาน เมื่อต้องทำรายงานประจำปีในอนาคต ให้กรองผลงานตามเกณฑ์รายงาน ณ ตอนนั้น และนับหน่วย `(user_id, document_id)` เพื่อให้แต่ละอาจารย์ได้รับบทบาทของตน

## สิ่งที่ตรวจพบจากข้อมูลจริง

- Scopus Search ที่ระบบ ingest อยู่มีรายชื่อผู้เขียนตามลำดับ แต่ไม่มี field ที่ยืนยัน corresponding หรือ co-first; `author_seq` เดิมสร้างจากตำแหน่งใน Search `author[]` ส่วน Abstract Retrieval XML `view=FULL` มี `<author seq="1">` และ `dc:creator` ใช้ตรวจผู้เขียนคนแรก
- ในตัวอย่างผลงานคณะ 52 ชิ้นที่ตรวจ XML ก่อนเริ่มงาน: 33 ชิ้นมี `<correspondence>` หนึ่งคน, 3 ชิ้นมีหลายคน, 16 ชิ้นไม่มีส่วนนี้ ไม่พบ marker co-first ใน XML ตัวอย่าง
- Scopus ID `85160859691` มี `<correspondence>` สองคนคือ Surasak Tangsakul และ Sartra Wongthanavasu; [หน้าสำนักพิมพ์](https://www.mdpi.com/2076-3417/13/10/6081) ระบุทั้งสองคนเป็น corresponding XML บางครั้งให้ `author-instance-id` ใน `<person>` ต่างจาก `<author>` จึงจับคู่ด้วย Scopus Author ID ก่อน และใช้ชื่อที่ตรงแบบไม่กำกวมสำหรับ `<person>` ที่ไม่มี ID
- Scopus ID `85091954319` มีข้อมูล [สำนักพิมพ์](https://www.mdpi.com/2076-3417/10/18/6381) ว่าผู้เขียนสองคนมีส่วนร่วมเท่ากัน แต่ XML ไม่ส่ง marker นี้ ดังนั้นรุ่นนี้ยังแยก co-first ไม่ได้
- บางผลงาน Scopus Author ID ใน XML ต่างจาก Search JSON ที่ ingest ไว้ แม้ชื่อและนามสกุลตรงกัน เช่นเอกสารภายในหมายเลข 570/573/681 จึงใช้ชื่อเต็มที่ตรงและไม่ซ้ำภายในผลงานเป็นทางสำรอง หากกำกวมให้ `needs_review`
- เอกสารภายในหมายเลข 177 เคยมีลิงก์ผู้เขียน Xuesong Zhang สอง ID: `60039188600` (แถวเก่า) และ `55715512900` (ID ที่ Search JSON และ XML ปัจจุบันระบุตรงกัน) ผู้ใช้ตรวจหน้า Scopus แล้วพบว่า [ID เก่า](https://www.scopus.com/authid/detail.uri?authorId=60039188600) redirect ไป [ID ปัจจุบัน](https://www.scopus.com/authid/detail.uri?authorId=55715512900) ซึ่งสอดคล้องกับแนวคิด Author Profile ที่ถูกแทนที่/รวมโปรไฟล์ใน [คู่มือ API ของ Elsevier](https://dev.elsevier.com/guides/Scopus%20API%20Guide_V1_20230907.pdf) เครื่องมืออัตโนมัติเปิดหน้า Scopus ได้ HTTP 403 จึงยังไม่ได้ยืนยัน redirect นี้จากเครื่องมือ และไม่อาจระบุสาเหตุภายใน Scopus ของคู่นี้ได้แน่ชัด
- การไม่มี `<correspondence>` **ไม่ได้พิสูจน์** ว่าไม่มี corresponding author; ตามกติกาที่ตกลงสำหรับการใช้งานนี้ ผู้เขียนลำดับอื่นที่ XML ไม่ระบุ corresponding จะจัดเป็น co-author
- บาง XML มี `<correspondence>` ที่ระบุเพียง affiliation โดยไม่มี `<person>`; กรณีนี้ไม่มีชื่อให้จับคู่และใช้กติกาเดียวกับ XML ที่ไม่ระบุ corresponding
- ขณะตรวจฐานข้อมูลแบบอ่านอย่างเดียววันที่ 29 กันยายน 2026 พบผลงาน Scopus หลักที่ตรงเกณฑ์ 949 ชิ้น จำนวนจริงอาจเปลี่ยนก่อนรัน

## ที่เก็บและกติกา

Migration `047_20260929_add_scopus_author_roles.sql` เพิ่ม `scopus_documents.author_role_status`, `author_role_checked_at` และ flags แบบ nullable `scopus_document_authors.is_first_author`, `is_corresponding_author` ค่า `NULL` คือยังสรุปไม่ได้; `0` คือผลจาก XML ที่ตรวจสำเร็จว่าไม่อยู่ในบทบาทนั้น; `1` คืออยู่ในบทบาทนั้น

สถานะของผลงาน:

| status | ความหมาย | backfill ปกติ |
|---|---|---|
| `NULL` | ยังไม่ตรวจ | ดึง |
| `complete` | จับคู่ผู้เขียนและ correspondence ครบ | ข้าม |
| `no_correspondence` | จับคู่ผู้เขียนครบ แต่ XML ไม่มี correspondence | ข้าม |
| `needs_review` | XML สำเร็จแต่โครงสร้าง/การจับคู่ไม่ชัด | ข้าม; ใช้ `-retry-review` หลังแก้ไข |
| `fetch_error` | ขอ XML ไม่สำเร็จ; `author_role_checked_at` ยังเป็น `NULL` | ลองใหม่ |

First author มาจาก XML `author seq="1"` และตรวจทานกับ `dc:creator` เมื่อมี Scopus Author ID; corresponding มาจาก `<correspondence><person>` ทุกคน บุคคลเดียวมีสอง flag เป็น `1` ได้ และหลายคนมี corresponding flag เป็น `1` ได้ โดยยังไม่มี flag แยก co-corresponding Co-author เป็นค่าที่คำนวณจาก **ทั้งสอง flag เป็น `0`** บนผลงานสถานะ `complete` หรือ `no_correspondence`; ไม่เก็บเป็นคอลัมน์ซ้ำ

หาก XML มี correspondence แต่จับคู่ไม่ได้อย่างแน่ชัด จะตั้ง `needs_review` และปล่อย flags เป็น `NULL` ทุกคน เพื่อไม่แสดง co-author ที่อาจผิด จับคู่ XML author ด้วย Scopus Author ID ก่อน และเมื่อ ID ไม่ตรงใช้ given name + surname ที่ตรงกันแบบไม่กำกวมเท่านั้น งาน ingest Search เดิมยังเป็นแหล่งข้อมูลชื่อและ affiliation และไม่เรียก XML เอง; การ ingest ซ้ำรักษา flags เดิมไว้ หากลำดับหรือรายชื่อผู้เขียนใน Search เปลี่ยน จะล้าง flags และสถานะให้ backfill รอบถัดไปตรวจใหม่

ข้อมูลเก่าบางชิ้นมีลิงก์ผู้เขียนค้างจาก Scopus Author ID ที่เปลี่ยนไป งาน backfill ลบลิงก์ค้างได้เฉพาะกรณีที่รายชื่อ Scopus Author ID จาก Search JSON ที่บันทึกไว้ **ตรงกันทั้งหมด** กับ XML ปัจจุบัน ทั้งสองแหล่งไม่พบ ID ของลิงก์ค้าง และลิงก์ค้างนั้น **ไม่เป็น Scopus ID ของผู้ใช้ใน `users`** หากหลักฐานไม่ครบจะคงข้อมูลไว้และใช้ `needs_review` เพื่อไม่ทำให้ผลงานหายจากเจ้าของ

## วิธีรัน

1. ใช้ migration ตามลำดับเลข โดยรัน `047_20260929_add_scopus_author_roles.sql` หลัง `046` บนฐานข้อมูลเป้าหมาย
2. ตรวจว่า `scopus_config` มี key `X-ELS-APIKey` หรือ `api_key` และ VPN ติดต่อ Scopus ได้ คำสั่งใช้ key ผ่าน HTTP header ไม่ใส่ใน URL หรือเอกสาร
3. ใน `fund-management-api` รัน `go run ./cmd/scopus-author-roles -limit 10` เพื่อทดลอง แล้วรัน `go run ./cmd/scopus-author-roles` เพื่อเติมที่เหลือ คำสั่งคืน summary JSON และบันทึก ID ของรายการมีปัญหาใน log
4. รันคำสั่งเดิมหลังงาน ingest รอบใหม่ได้ รายการที่มี `author_role_checked_at` แล้วถูกข้าม; ใช้ `-retry-review` ตรวจซ้ำเฉพาะรายการที่เคยจับคู่ไม่ได้ หรือ `-refresh` เมื่อต้องการตรวจใหม่ทั้งชุด (ใช้ `-limit` จำกัดชุดได้) หากได้ HTTP 429/401/403 งานหยุดเพื่อไม่ยิงต่อ และรันซ้ำได้ภายหลัง

จำนวนคำขอใหม่ = จำนวนผลงานที่ยังไม่ตรวจและถูกเลือก ไม่ใช่จำนวนอาจารย์หรือจำนวนแถวผู้เขียน ไม่ต้องเรียก Search เพิ่ม งาน `-refresh` จะเรียกอีกครั้งต่อผลงานที่เลือก

ระหว่างงานนี้แก้ request log ของ Search ingest ให้ตัด `X-ELS-APIKey` ออกจาก headers ที่บันทึกใหม่ด้วย; ไม่เปลี่ยนข้อมูล log เก่าที่มีอยู่ก่อนหน้า

## ตรวจผลและข้อจำกัด

หลังรันให้เทียบ `eligible`, `selected`, `fetched`, `complete`, `no_correspondence`, `needs_review`, `failed` จาก summary กับ DB; สุ่มเปิดเอกสารที่ first/corresponding ซ้อนกัน, หลาย corresponding และไม่มี correspondence เพื่อตรวจ flags หากมี `needs_review` ให้ตรวจ XML และข้อมูลผู้เขียนท้องถิ่นก่อน refresh

**ผลการรันย้อนหลังจริง (29 กันยายน 2026):** ลง migration 047 แล้ว งาน backfill ตรวจครบ 949 ผลงานที่เข้าเกณฑ์: `complete` 671, `no_correspondence` 278, `needs_review` 0, `fetch_error` 0 รอบแก้ไข `-retry-review` ปิดได้ 13/13 รายการ และลบลิงก์ผู้เขียนเก่าค้าง 11 แถวหลัง Search กับ XML ยืนยันตรงกัน รอบรันปกติซ้ำได้ `selected=0`, `fetched=0`, `skipped_existing=949`

ตรวจ DB หลังรัน: ไม่มีผู้เขียนของผลงานที่ตรวจแล้วซึ่ง flag ยังเป็น `NULL`; ทุกผลงานมี first author หนึ่งคน; ทุกผลงานสถานะ `complete` มี corresponding อย่างน้อยหนึ่งคน; สถานะ `no_correspondence` ไม่มี flag corresponding; 58 ผลงานมี corresponding มากกว่าหนึ่งคน ตัวอย่าง Scopus ID `85160859691` เก็บ corresponding ทั้งสองคน และ Scopus ID `85091954319` เก็บผู้เขียนลำดับแรกกับ corresponding ลำดับสอง โดยยังไม่อ้างว่าได้แยก co-first

ตรวจการเก็บบทบาทคนนอกคณะด้วย Scopus ID `85126809240`: ผู้เขียนในคณะเป็น co-author ส่วน first (`56562406000`) และ corresponding (`7801622743`) เป็นผู้เขียนนอกคณะและมี flags อยู่ใน DB

รุ่นนี้ยังไม่ยืนยัน co-first จาก XML และไม่แยก co-corresponding เป็นป้ายเฉพาะ การกรองผลงานตาม affiliation, ปีจ้าง หรือเกณฑ์รายงานผู้บริหาร เป็นหน้าที่ของ query รายงานในอนาคต
