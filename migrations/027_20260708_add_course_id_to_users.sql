-- เพิ่มคอลัมน์ course_id ให้ users (อ้างอิงหลักสูตร instructor_courses แบบหลวม ๆ ไม่บังคับ FK)
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS course_id int(11) DEFAULT NULL;
