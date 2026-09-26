# การจัดหมวดบทความ Scopus แบบชุด

รัน `migrations/047_20260926_scopus_bulk_classification.sql` **ในฐานข้อมูล fund-management ที่ถูกต้อง** หลัง migration 045 และ 046 ก่อนเปิด API เวอร์ชันนี้ หน้า Admin → นำเข้าผลงานวิชาการ → Scopus จะแสดงส่วนจัดหมวดแยกข้อมูลระดับประเทศ (`scopus_benchmark_documents`) กับข้อมูลของคณะ (`scopus_documents`)

Backend ต้องตั้ง `PAPER_CLASSIFICATION_API_URL` และ `PAPER_AI_API_KEY` ให้เรียก Classification API ของ academic-insight-ai ได้ งานชุดใช้โมเดล `qwen3-4b` พร้อม `fewshot_candidate` และ `confidence_policy: model_reported` โดยส่งชื่อบทความ บทคัดย่อ และคำสำคัญ ไม่มีการอ่าน PDF หรือเปิด Scopus link

เมื่อต้องรัน backend ในเครื่องเพื่อทดสอบกับฐานข้อมูลเทส ตั้ง `DISABLE_MOU_NOTIFICATION_SCHEDULER=true` ใน process นั้นเพื่อป้องกันการส่งอีเมลแจ้งเตือน MOU ที่ไม่เกี่ยวกับการทดสอบ งานแจ้งเตือนจะทำงานตามปกติถ้าไม่ได้ตั้งค่านี้

เลือกปีหรือปล่อยว่างสำหรับทุกปี แล้วเลือกเฉพาะรายการที่ยังไม่จัดหมวด (ค่าเริ่มต้น) หรือจัดหมวดใหม่ทั้งหมด ระบบจะแสดงจำนวนก่อนเริ่ม งานหนึ่งรายการทำงานได้ครั้งละหนึ่งชุดข้อมูล เมื่อหยุดจะจบหลังบทความที่กำลังประมวลผล สามารถดำเนินการต่อหรือทดลองรายการที่ล้มเหลวอีกครั้งได้ หาก server รีสตาร์ต งานที่ค้างจะมีสถานะ `interrupted` และดำเนินการต่อได้จากหน้าเดิม

ผลล่าสุดเขียนลง `category`, `classification_confidence`, `classification_model`, `classification_taxonomy_version`, `classified_at` ของตารางต้นทาง `Preface` จะเก็บ `category = NULL` ผลก่อนหน้าและคำตอบ AI อยู่ใน `paper_classification_run_items` เพื่อใช้ตรวจย้อนหลัง หากบริการ AI ล้มเหลวหรือข้อมูลบทความเปลี่ยนระหว่างคิวและบันทึกผล รายการนั้นจะเป็น `failed` และไม่แก้ค่าเดิม ระบบหยุดอัตโนมัติหลังล้มเหลวติดต่อกันสามรายการเพื่อหลีกเลี่ยงการส่งคำขอจำนวนมากไปยังบริการที่อาจไม่พร้อม

`classification_confidence` ของงานใหม่เป็นระดับที่โมเดลระบุเอง (`High`, `Medium`, `Low` หรือ `Preface`) ไม่ใช่ค่าความน่าจะเป็นที่ผ่านการสอบเทียบ การประเมินก่อนหน้าพบว่าโมเดลให้ `High` เกือบทุกบทความ แม้บางหมวดต่างจากชุดอ้างอิง การเปลี่ยนนโยบายนี้ไม่แก้ผลเก่าที่บันทึกเป็น `Medium`; หากต้องการประเมินใหม่ต้องสั่งงานขอบเขต `all` โดยทราบว่าจะเขียนทับผลล่าสุดและเก็บผลเดิมในประวัติงาน

API สำหรับ Postman (ใช้ bearer token ของผู้ดูแล):

1. `GET /api/v1/admin/paper-ai/classification/years?source=benchmark` ดูปีที่มีบทความและจำนวนทั้งหมดของแต่ละปี
2. `GET /api/v1/admin/paper-ai/classification/preview?source=benchmark&scope=unprocessed&year=2025&page=1` (ละ `year` ได้) คืนจำนวนและรายชื่อบทความหน้าละ 20 รายการ
3. `POST /api/v1/admin/paper-ai/classification/runs` ส่ง JSON `{"source":"benchmark","scope":"unprocessed","year":2025}` (`year: null` หมายถึงทุกปี)
4. `GET /api/v1/admin/paper-ai/classification/runs` และ `GET /api/v1/admin/paper-ai/classification/runs/{run_id}`
5. `GET /api/v1/admin/paper-ai/classification/runs/{run_id}/items?page=1` ดูผลรายบทความ
6. `POST /api/v1/admin/paper-ai/classification/runs/{run_id}/stop`, `/resume`, หรือ `/retry-failed` ตามสถานะงาน

ก่อนใช้งานจริง ควรทดลองหนึ่งปีที่มีข้อมูลไม่มาก ตรวจผล `Preface` และ `failed` ในหน้าประวัติ แล้วค่อยเลือกทุกปี Migration เปลี่ยน schema และยังไม่ได้ถูกรันโดยโค้ดนี้
