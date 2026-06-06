-- เพิ่มคอลัมน์ใหม่เข้าไปก่อน
ALTER TABLE users ADD COLUMN course_id INT;

-- สร้างข้อกำหนด Foreign Key เชื่อมไปยังตาราง courses
ALTER TABLE users 
ADD CONSTRAINT fk_instructor_course
FOREIGN KEY (course_id) REFERENCES instructor_courses(course_id)
ON DELETE SET NULL -- ถ้าลบหลักสูตร ให้ค่าใน user เป็น NULL (หรือใช้ RESTRICT ตามความเหมาะสม)
ON UPDATE CASCADE;