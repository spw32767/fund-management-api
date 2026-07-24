CREATE TABLE IF NOT EXISTS sdgs (
  sdg_id INT NOT NULL AUTO_INCREMENT,
  sdg_number TINYINT UNSIGNED NOT NULL,
  name_th VARCHAR(255) NOT NULL,
  name_en VARCHAR(255) NOT NULL,
  description_th TEXT DEFAULT NULL,
  description_en TEXT DEFAULT NULL,
  create_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  delete_at DATETIME DEFAULT NULL,
  PRIMARY KEY (sdg_id),
  UNIQUE KEY uq_sdgs_number (sdg_number),
  KEY idx_sdgs_delete_at (delete_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO sdgs
  (sdg_number, name_th, name_en, description_th, description_en)
VALUES
  (1, 'ขจัดความยากจน', 'No Poverty', 'ยุติความยากจนทุกรูปแบบในทุกพื้นที่', 'End poverty in all its forms everywhere'),
  (2, 'ขจัดความหิวโหย', 'Zero Hunger', 'ยุติความหิวโหย บรรลุความมั่นคงทางอาหารและโภชนาการที่ดีขึ้น และส่งเสริมเกษตรกรรมที่ยั่งยืน', 'End hunger, achieve food security and improved nutrition, and promote sustainable agriculture'),
  (3, 'สุขภาพและความเป็นอยู่ที่ดี', 'Good Health and Well-being', 'สร้างหลักประกันการมีสุขภาวะที่ดีและส่งเสริมความเป็นอยู่ที่ดีสำหรับทุกคนในทุกวัย', 'Ensure healthy lives and promote well-being for all at all ages'),
  (4, 'การศึกษาที่มีคุณภาพ', 'Quality Education', 'สร้างหลักประกันการศึกษาที่มีคุณภาพอย่างครอบคลุมและเท่าเทียม และสนับสนุนโอกาสการเรียนรู้ตลอดชีวิตสำหรับทุกคน', 'Ensure inclusive and equitable quality education and promote lifelong learning opportunities for all'),
  (5, 'ความเท่าเทียมทางเพศ', 'Gender Equality', 'บรรลุความเท่าเทียมทางเพศและเสริมพลังให้แก่ผู้หญิงและเด็กหญิงทุกคน', 'Achieve gender equality and empower all women and girls'),
  (6, 'น้ำสะอาดและการสุขาภิบาล', 'Clean Water and Sanitation', 'สร้างหลักประกันเรื่องน้ำและการสุขาภิบาลให้มีการจัดการอย่างยั่งยืนและมีสภาพพร้อมใช้สำหรับทุกคน', 'Ensure availability and sustainable management of water and sanitation for all'),
  (7, 'พลังงานสะอาดที่เข้าถึงได้', 'Affordable and Clean Energy', 'สร้างหลักประกันให้ทุกคนเข้าถึงพลังงานสมัยใหม่ที่ยั่งยืนในราคาที่เข้าถึงได้', 'Ensure access to affordable, reliable, sustainable and modern energy for all'),
  (8, 'งานที่มีคุณค่าและการเติบโตทางเศรษฐกิจ', 'Decent Work and Economic Growth', 'ส่งเสริมการเติบโตทางเศรษฐกิจที่ต่อเนื่อง ครอบคลุม และยั่งยืน การจ้างงานเต็มที่และมีผลิตภาพ และการมีงานที่มีคุณค่าสำหรับทุกคน', 'Promote sustained, inclusive and sustainable economic growth, full and productive employment and decent work for all'),
  (9, 'อุตสาหกรรม นวัตกรรม และโครงสร้างพื้นฐาน', 'Industry, Innovation and Infrastructure', 'สร้างโครงสร้างพื้นฐานที่มีความยืดหยุ่น ส่งเสริมการพัฒนาอุตสาหกรรมที่ครอบคลุมและยั่งยืน และส่งเสริมนวัตกรรม', 'Build resilient infrastructure, promote inclusive and sustainable industrialization and foster innovation'),
  (10, 'ลดความเหลื่อมล้ำ', 'Reduced Inequalities', 'ลดความไม่เสมอภาคภายในประเทศและระหว่างประเทศ', 'Reduce inequality within and among countries'),
  (11, 'เมืองและชุมชนที่ยั่งยืน', 'Sustainable Cities and Communities', 'ทำให้เมืองและการตั้งถิ่นฐานของมนุษย์มีความครอบคลุม ปลอดภัย ยืดหยุ่น และยั่งยืน', 'Make cities and human settlements inclusive, safe, resilient and sustainable'),
  (12, 'การผลิตและการบริโภคที่รับผิดชอบ', 'Responsible Consumption and Production', 'สร้างหลักประกันให้มีรูปแบบการผลิตและการบริโภคที่ยั่งยืน', 'Ensure sustainable consumption and production patterns'),
  (13, 'การรับมือกับการเปลี่ยนแปลงสภาพภูมิอากาศ', 'Climate Action', 'ปฏิบัติการอย่างเร่งด่วนเพื่อต่อสู้กับการเปลี่ยนแปลงสภาพภูมิอากาศและผลกระทบที่เกิดขึ้น', 'Take urgent action to combat climate change and its impacts'),
  (14, 'ทรัพยากรทางทะเล', 'Life Below Water', 'อนุรักษ์และใช้ประโยชน์จากมหาสมุทร ทะเล และทรัพยากรทางทะเลอย่างยั่งยืนเพื่อการพัฒนาที่ยั่งยืน', 'Conserve and sustainably use the oceans, seas and marine resources for sustainable development'),
  (15, 'ระบบนิเวศบนบก', 'Life on Land', 'ปกป้อง ฟื้นฟู และส่งเสริมการใช้ระบบนิเวศบนบกอย่างยั่งยืน จัดการป่าไม้อย่างยั่งยืน ต่อต้านการแปรสภาพเป็นทะเลทราย หยุดยั้งความเสื่อมโทรมของที่ดิน และหยุดยั้งการสูญเสียความหลากหลายทางชีวภาพ', 'Protect, restore and promote sustainable use of terrestrial ecosystems, sustainably manage forests, combat desertification, halt land degradation and halt biodiversity loss'),
  (16, 'สันติภาพ ความยุติธรรม และสถาบันที่เข้มแข็ง', 'Peace, Justice and Strong Institutions', 'ส่งเสริมสังคมที่สงบสุขและครอบคลุมเพื่อการพัฒนาที่ยั่งยืน ให้ทุกคนเข้าถึงความยุติธรรม และสร้างสถาบันที่มีประสิทธิผล รับผิดชอบ และครอบคลุมในทุกระดับ', 'Promote peaceful and inclusive societies for sustainable development, provide access to justice for all and build effective, accountable and inclusive institutions at all levels'),
  (17, 'ความร่วมมือเพื่อการพัฒนาที่ยั่งยืน', 'Partnerships for the Goals', 'เสริมความเข้มแข็งให้แก่กลไกการดำเนินงานและฟื้นฟูความร่วมมือระดับโลกเพื่อการพัฒนาที่ยั่งยืน', 'Strengthen the means of implementation and revitalize the global partnership for sustainable development');
