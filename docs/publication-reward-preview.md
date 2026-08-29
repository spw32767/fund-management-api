# Publication Reward — เอกสารตัวอย่าง/รวม (docx → PDF)

เอกสารนี้อธิบายหลักการทำงานของฟีเจอร์ **"ดูตัวอย่างเอกสารรวม"** ในหน้าขอทุน โดยเฉพาะ
ส่วนที่ **สร้างไฟล์ .docx จาก template แล้วแปลงเป็น PDF** เขียนไว้ให้ทั้ง dev และ AI agent
อ่านแล้วเข้าใจได้โดยไม่ต้องไล่โค้ดใหม่

> อัปเดตล่าสุด: 2026-08-16 · โค้ดหลัก: [`controllers/publication_reward_preview.go`](../controllers/publication_reward_preview.go)

---

## 1. ภาพรวม: มี 2 เส้นทางที่ไม่เหมือนกัน

ปุ่ม "ดูตัวอย่างเอกสารรวม" มีอยู่ใน 2 ฟอร์ม และ**ทำงานคนละแบบ** — อย่าสับสน:

| ฟอร์ม | ไฟล์ frontend | กลไก | มี docx ไหม |
|---|---|---|---|
| **เงินรางวัลตีพิมพ์** (Publication Reward) | `PublicationRewardForm.js` | เรียก backend สร้าง **PDF ปกจาก docx template** แล้ว merge ไฟล์แนบ | ✅ ใช่ (เอกสารนี้พูดถึงตัวนี้) |
| **ทุนทั่วไป** (Generic Fund) | `GenericFundApplicationForm.js` | รวมเฉพาะไฟล์แนบ PDF **ฝั่ง browser** ด้วย `pdf-lib` | ❌ ไม่มี template/ไม่มี backend |

เอกสารนี้โฟกัสเส้นทาง **Publication Reward** (เส้นที่สร้าง docx จริง)

---

## 2. Flow ฝั่ง Publication Reward

```
[ผู้ใช้กดปุ่ม] PublicationRewardForm.js → generatePreview()
   │  POST multipart/form-data  { data: JSON payload, attachments: [PDF...] }
   ▼
POST /api/v1/publication-summary/preview          (routes/routes.go)
   → controllers.PreviewPublicationReward         (publication_reward_preview.go)
       ├─ multipart?  → handlePublicationRewardPreviewForm   (จากฟอร์ม, ยังไม่ submit)
       └─ JSON?       → handlePublicationRewardPreviewSubmission (จาก submission ที่บันทึกแล้ว)
   ▼
1) build replacements map  (map[string]string ของ {{placeholder}} → ค่า)
2) fillDocxTemplate(templatePath, out.docx, replacements)   ← เติมค่าลง template
3) generatePublicationRewardPDF(replacements)               ← LibreOffice แปลง docx→PDF
4) (form path เท่านั้น) mergePreviewPDFWithAttachments()    ← ต่อไฟล์แนบ PDF ท้ายปก
   ▼
ส่ง application/pdf กลับ → frontend เปิดใน tab ใหม่
```

จุดที่ต้องรู้:
- **2 handler ต้อง sync กันเสมอ** — ถ้าเพิ่ม placeholder ต้องเพิ่มทั้ง `handlePublicationRewardPreviewForm`
  (ผ่าน `buildFormPreviewReplacements`) **และ** `handlePublicationRewardPreviewSubmission`
- Form path ใช้ค่าจาก payload ที่ผู้ใช้กรอก; Submission path ใช้ค่าจาก DB (`models.PublicationRewardDetail`)

---

## 3. การเติม template — `fillDocxTemplate`

.docx คือ zip ของไฟล์ XML การเติมค่าทำแบบนี้ ([ดูฟังก์ชัน](../controllers/publication_reward_preview.go)):

1. เปิด template.docx เป็น zip, วนอ่านทุก entry
2. เฉพาะไฟล์ `.xml` → `normalizeDocxPlaceholders()` แล้ว `strings.ReplaceAll` ทีละ placeholder
3. เขียน entry กลับเป็น zip ใหม่ (out.docx)

### ⚠️ กับดักสำคัญ: placeholder ถูก Word ตัดข้าม run
Word มักเก็บ `{{external_fund_list}}` เป็นหลายชิ้นข้าม `<w:r>`/`<w:t>` เช่น `{{external_` + `fund_list` + `}}`
ทำให้ `ReplaceAll` หาไม่เจอ → placeholder โผล่ดิบใน PDF

**ตัวช่วย:** `normalizeDocxPlaceholders()` + `placeholderRegexFor()` สร้าง regex ที่ยอมให้มี tag/space
คั่นระหว่างตัวอักษร แล้วรวม placeholder กลับให้ก่อน replace (และลบ `<w:proofErr/>` ทิ้ง)

**กฎเวลาแก้ template:** พิมพ์ placeholder รวดเดียว อย่า format กลางคำ / paste แบบ plain
เพื่อให้มันอยู่ใน run เดียว → replace ติดชัวร์ (ตารางสรุปที่เราสร้าง placeholder ทุกตัวอยู่ run เดียวอยู่แล้ว)

---

## 4. การแปลง PDF — `generatePublicationRewardPDF`

- หา binary ด้วย `lookupLibreOfficeBinary()` → อ่าน env **`LIBREOFFICE_PATH`** (ตั้งใน `.env`;
  prod = `C:/Program Files/LibreOffice/program/soffice.exe`) หรือหา `soffice`/`libreoffice` ใน PATH
- `configureLibreOfficeFonts()` สร้าง fontconfig ชี้ไป `templates/fonts` (TH Sarabun New) +
  `frontend_project_fund/public/font` และ map ชื่อฟอนต์ (Cordia/Angsana → TH Sarabun New ฯลฯ)
- รัน `soffice -env:UserInstallation=<profile> --headless --convert-to
  pdf:writer_pdf_Export:EmbedStandardFonts=true;EmbedFonts=true --outdir <tmp> <docx>`

> path ทั้งหมด **relative กับ cwd = repo root** (`templates/...`, `frontend_project_fund/...`)
> เวลารันเทสต์ต้อง `os.Chdir("..")` ไป repo root ก่อน ไม่งั้นหา fonts/template ไม่เจอ

Merge ไฟล์แนบ (form path): `mergePreviewPDFWithAttachments()` → `mergePDFs()` ลองตามลำดับ
**Node+pdf-lib** (`scripts/merge_pdf.js`, ต้องเจอ `../frontend_project_fund/node_modules`) →
ghostscript (`gs`) → `pdfunite` ไฟล์ที่ไม่ใช่ PDF จะถูก skip

---

## 5. Templates

อยู่ใน [`templates/`](../templates/):

| ไฟล์ | สถานะ |
|---|---|
| `publication_reward_template.docx` | **ตัวหลักที่ prod ใช้จริง = ดีไซน์ใหม่แล้ว** (ตารางสรุป +/-) — code (`generatePublicationRewardPDF`, `renderPublicationRewardDocx`) hardcode ชื่อนี้ |
| `publication_reward_template_backup.docx` | **ดีไซน์เก่า** เก็บไว้เป็น backup ไม่ถูกเรียกในโค้ด |
| `publication_reward_template_with_dept.docx` | ของเก่า orphan ไม่ถูกเรียกที่ไหน อย่าแตะ |

### Deploy
ดีไซน์ใหม่**อยู่ในไฟล์ชื่อ canonical แล้ว** (ทำ rename ใน git ตั้งแต่บน branch) → deploy = **merge branch อย่างเดียว** ไม่ต้อง rename/แตะไฟล์บน server
- rollback: `git revert` commit rename หรือดึง backup กลับ
- ต้อง apply migration `038_..._has_received_reward` (ของ main) และมี LibreOffice/fonts/Node บน prod

> ⚠️ **placeholder ต้อง sync 3 ที่เสมอ** — เพิ่ม placeholder ในตารางต้องเซ็ตค่าใน builder ทั้ง 3:
> `handlePublicationRewardPreviewSubmission` + `buildFormPreviewReplacements` (preview.go) และ
> `buildSubmissionPreviewReplacements` (submission.go = ตัวสร้างไฟล์จริงตอน submit) มิฉะนั้นไฟล์จริงจะมี `{{...}}` ดิบ

---

## 6. Placeholders

สร้างใน `handlePublicationRewardPreviewSubmission` และ `buildFormPreviewReplacements`
**ค่าที่ส่งมา format แล้ว** (มี comma, ทศนิยม 2 ตำแหน่ง) — ใน template อย่า format ซ้ำ

| Placeholder | ความหมาย | ที่มา (form / submission) |
|---|---|---|
| `{{applicant_name}}` | ชื่อผู้ขอ | payload.Applicant / submission.User |
| `{{date_th}}`, `{{date_of_employment}}`, `{{position}}` | วันที่/บรรจุ/ตำแหน่ง | payload / DB |
| `{{installment}}`, `{{kku_report_year}}` | งวด / ปีรายงาน | system_config + logic งวด |
| `{{paper_title}}`, `{{journal_name}}`, `{{publication_year}}`, `{{volume_issue}}`, `{{page_number}}` | ข้อมูลบทความ | payload / detail |
| `{{author_name_list}}`, `{{author_role}}`, `{{quartile_line}}` | ผู้แต่ง / บทบาท / บรรทัดควอไทล์ (ข้อความยาว) | payload / detail |
| `{{reward_amount}}` | **เงินรางวัล** | PublicationReward / detail.RewardAmount |
| `{{quartile}}` | **โค้ดควอไทล์สั้น** เช่น `Q1`/`T5`/`TCI` (ใช้กับ "เงินรางวัล Q1") | `buildQuartileLabel()` จาก JournalQuartile / detail.Quartile |
| `{{reward_received_note}}` | **หมายเหตุต่อท้าย label เงินรางวัล** = `" (เคยขอเงินรางวัลแล้ว)"` เมื่อเคยขอ, `""` เมื่อไม่เคย → อธิบายว่าทำไม reward = 0.00 | `buildRewardReceivedNote()` จาก HasReceivedReward |
| `{{manuscript_amount}}` | (A) ค่าปรับปรุงบทความ | RevisionFee |
| `{{page_charge_amount}}` | (B) ค่าธรรมเนียมตีพิมพ์ | PublicationFee |
| `{{external_fund_list}}` | รายการทุนภายนอก (หลายบรรทัด, join ด้วย `\n` → `<w:br/>`) | external funds |
| `{{external_fund_block}}` | **เหมือน list แต่มี `\n` นำหน้าเมื่อมีทุน / เป็น "" เมื่อไม่มี** → ใช้ในเซลล์ (C) ให้ทุนขึ้นบรรทัดใหม่ตอนมี และไม่มีบรรทัดลอยตอนไม่มี | `buildExternalFundBlock()` |
| `{{external_fund_total}}` | รวมทุนภายนอก (บวก) | sum |
| `{{external_fund_total_negative}}` | **(C) รวมทุนภายนอกแบบวงเล็บ** เช่น `(50,000.00)` | `formatAmountParen()` |
| `{{net_topup_amount}}` | **เงินสมทบ (A+B−C) ดิบ ไม่ clamp** ติดลบได้ | `formatAmount(B + A − external)` |
| `{{total_amount}}` | รวมจำนวนเงิน | TotalAmount |
| `{{total_amount_text}}` | จำนวนเงินเป็นตัวอักษร (บาทถ้วน) | `utils.BahtText()` |
| `{{document_line}}` | รายการเอกสารแนบ (นับจำนวน) — **กรองไฟล์ auto-generated ออก** (ดูข้อ 11) | `buildDocumentLine()` |
| `{{end_of_contract}}`, `{{signature}}` | ท้ายสัญญา / ลายเซ็น | DB / payload |

> **หมายเหตุ placeholder เก่าที่เลิกใช้ในตารางใหม่:** `{{page_charge_manuscript_total}}` (รวม A+B)
> ไม่มีในดีไซน์ใหม่แล้ว (ยังมีในโค้ด backend แต่ template ไม่ได้ใช้ — ไม่เป็นไร)

---

## 7. ตารางสรุป "รายการการขอเบิก/จ่าย" (ดีไซน์ใหม่)

โครงตาราง 2 คอลัมน์ (รายการ | จำนวนเงิน) mirror การคำนวณในเว็บ:

| แถว | placeholder | หมายเหตุ |
|---|---|---|
| เงินรางวัล + ควอไทล์ | `{{reward_amount}}` (label = `เงินรางวัล {{quartile}}{{reward_received_note}}`) | เคยขอ → reward=0.00 + note "(เคยขอเงินรางวัลแล้ว)"; มีเส้นหนาคั่นด้านล่าง |
| (A) ค่าปรับปรุงบทความ | `{{manuscript_amount}}` | |
| (B) ค่าธรรมเนียมการตีพิมพ์ | `{{page_charge_amount}}` | |
| หัก (C) เงินสนับสนุนภายนอก | `{{external_fund_total_negative}}` (แดง) + `{{external_fund_block}}` | ย่อหน้าเยื้อง; ทุนขึ้นบรรทัดใหม่ใต้ label |
| เงินสมทบที่ขอเบิก (A + B − C) | `{{net_topup_amount}}` | แถวพื้นเทา |
| รวมจำนวนเงิน (Total Amount) | `{{total_amount}}` (น้ำเงิน) | เส้นหนาคั่นบน+ล่าง |

### Design decisions (สำคัญ — อย่าเผลอ "แก้ให้ถูก")
- **net top-up ไม่ clamp**: มิเรอร์แถว "เงินสมทบ (A+B−C)" ของเว็บ
  ([PublicationSubmissionDetails.js](../../frontend_project_fund/app/(portal)/research-fund-system/admin/components/submissions/PublicationSubmissionDetails.js)) ซึ่งแสดงผลต่างดิบ **ติดลบได้**
  (ต่างจากแถว Total ของเว็บที่ใช้ `Math.max(0, …)`) — ตั้งใจให้เป็นแบบนี้
- **ไม่มีคอลัมน์ +/-**: การ "หัก" สื่อผ่าน (1) ตัวเลขแดง (2) วงเล็บบัญชี (3) คำว่า "หัก" (4) สูตร "(A+B−C)"
- **`{{external_fund_block}}` ในเซลล์ (C)**: backend ใส่ `\n` นำหน้าเฉพาะตอนมีทุน →
  ทุนขึ้นบรรทัดใหม่ใต้ label เมื่อมี และไม่เหลือบรรทัดว่างลอยเมื่อไม่มี (แก้ทั้งสองเคสพร้อมกัน
  ซึ่ง template ล้วนทำไม่ได้ ต้องพึ่ง logic ฝั่ง backend)

---

## 8. วิธีแก้ template (ไม่มี Word ก็ทำได้)

เพราะไม่มี Word/`python-docx` บนเครื่อง dev เราแก้ผ่านสคริปต์ประกอบ XML:

1. ก็อป template หลัก → `..._new.docx`
2. unzip เอา `word/document.xml`
3. รัน Node script ประกอบ `<w:tbl>` แล้ว **splice** แทนย่อหน้าเดิม
   (anchor ด้วย `w14:paraId` — บล็อกเดิมคือ paraId `236D372B` … `79162154`)
4. validate XML (`[xml]` cast ใน PowerShell)
5. repack เฉพาะ entry `word/document.xml` กลับด้วย .NET `ZipArchive` (Update mode)
   — คงไฟล์อื่นไว้เป๊ะ ไม่ต้องมี `zip` CLI

> สคริปต์ตัวอย่างที่เคยใช้อยู่ใน scratchpad (`build_table3.js`) ไม่ได้ commit เข้า repo —
> ถ้าจะแก้ layout อีกให้เขียนใหม่ตามแพทเทิร์นนี้ วางฟอนต์เป็น `TH Sarabun New`, `w:sz=28` (14pt)

---

## 9. การทดสอบ

### Regression test (hermetic, ไม่ต้องมี LibreOffice — รันใน CI ได้)
ไฟล์: [`controllers/publication_reward_template_new_test.go`](../controllers/publication_reward_template_new_test.go)

```bash
go test ./controllers/ -run TestNewTemplate
```

ครอบคลุม: ไม่มี placeholder ตกค้าง, ยอด foots (reward+A+B−C), เคสไม่มีทุน → `(0.00)` และ net=A+B,
net ติดลบได้เมื่อ C เกิน A+B

### Render จริง (ต้องมี LibreOffice)
เทสต์ render เต็ม pipeline เป็นแบบ throwaway (ไม่ commit) — ถ้าจะสร้างไฟล์ตัวอย่าง ให้เขียน test ที่:
- `os.Chdir("..")` ไป repo root
- เรียก `fillDocxTemplate` + จำลอง body ของ `generatePublicationRewardPDF`
  (`configureLibreOfficeFonts` → `lookupLibreOfficeBinary` → exec soffice)
- ตั้ง env `LIBREOFFICE_PATH` และ `OUT_DIR`

```bash
OUT_DIR=/path/out LIBREOFFICE_PATH="C:/Program Files/LibreOffice/program/soffice.exe" \
  go test ./controllers/ -run <ชื่อ throwaway test> -v
```

---

## 10. ความเปราะบาง (รู้ไว้)

- **พึ่ง LibreOffice บน prod** — `LIBREOFFICE_PATH` ผิด = ทั้งฟีเจอร์พัง; spawn ใหม่ทุกครั้ง (ช้า)
- **พึ่ง Node + pdf-lib ผ่าน relative path** `../frontend_project_fund/node_modules` (form path merge)
- **fill = string-replace บน raw XML** — เปราะกับ placeholder ที่ Word ตัดข้าม run (มี normalize ช่วย)
- **ไม่มี cache** — กด preview ซ้ำ = แปลง docx→pdf ใหม่ทุกครั้ง
- ไฟล์ฟอร์ม frontend ใหญ่มาก (`PublicationRewardForm.js` ~8,700 บรรทัด)

---

## 11. `{{document_line}}` — กรองไฟล์ auto-generated ออก

**บั๊กที่เคยเจอ:** รายการ "ทั้งนี้ได้แนบ หลักฐาน..." มีไฟล์ที่ระบบสร้างเองหลัง submit หลุดเข้ามา
เฉพาะ**คนที่ส่งซ้ำ (ใบที่ถูกตีกลับ)** ได้แก่:
- `แบบฟอร์มคำขอรับเงินรางวัล (DOCX) (Auto Generated)` — code `publication_reward_form_docx`
- `แบบฟอร์มคำขอรับเงินรางวัล (PDF) (Auto Generated)` — code `publication_reward_form_pdf`
- `แบบฟอร์มคำร้องรวม (merged pdf)` — code = ชื่อไทยนั้นเอง

**สาเหตุ:** ตอน re-submit, `fetchSubmissionDocuments()` ดึงเอกสารทั้งหมด **ก่อน**
`deletePreviousGeneratedFormDocuments()` จะลบตัวเก่า ทำให้ generated docs รอบก่อนยังอยู่ตอนสร้าง
`{{document_line}}` (ครั้งแรกที่ส่งไม่มีของเก่าเลยไม่หลุด)

**การแก้:** `buildDocumentLine()` เรียก `isGeneratedFormDocument()` ข้ามเอกสารที่ `DocumentType.Code`
ตรงกับ `generatedFormDocumentCodes` (3 code ข้างบน) — แก้จุดเดียวครอบคลุมทั้ง submit-time และ preview
เพราะทั้งคู่ใช้ `buildDocumentLine()` ร่วมกัน
เทสต์: `TestBuildDocumentLine_ExcludesGeneratedForms`
