package controllers

import (
	"archive/zip"
	"fund-management-api/utils"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// newTemplateForTest is the redesigned template. Path is relative to this package
// directory; fillDocxTemplate opens it directly, so no working-directory change is
// needed (unlike the LibreOffice PDF step, which is intentionally not exercised here
// so these tests stay hermetic and CI-friendly).
// The redesigned fee-summary template is now the canonical production file (the old
// design is kept as publication_reward_template_backup.docx).
const newTemplateForTest = "../templates/publication_reward_template.docx"

var placeholderPattern = regexp.MustCompile(`\{\{[a-z_]+\}\}`)

// publicationRewardSampleReplacements builds a full replacement map for the new
// template, deriving the external list/total, net top-up (A+B-C, unclamped) and grand
// total exactly the way the controller does.
func publicationRewardSampleReplacements(reward, manuscript, pageCharge float64, funds []PublicationRewardPreviewExternal) map[string]string {
	externalList, externalTotal := buildExternalFundLinesFromPreview(funds)
	total := reward + manuscript + pageCharge - externalTotal
	return map[string]string{
		"{{date_th}}":                      "15 สิงหาคม 2569",
		"{{applicant_name}}":               "ผศ.ดร. ทดสอบ ระบบ",
		"{{date_of_employment}}":           "1 มกราคม 2560",
		"{{position}}":                     "อาจารย์",
		"{{installment}}":                  "1",
		"{{total_amount}}":                 formatAmount(total),
		"{{total_amount_text}}":            utils.BahtText(total),
		"{{author_name_list}}":             "ทดสอบ ระบบ, John Doe",
		"{{paper_title}}":                  "A Study of Testing Systems",
		"{{journal_name}}":                 "Journal of Testing",
		"{{publication_year}}":             "2568",
		"{{volume_issue}}":                 "12(3)",
		"{{page_number}}":                  "100-120",
		"{{author_role}}":                  buildAuthorRole("first_author"),
		"{{quartile}}":                     buildQuartileLabel("Q1"),
		"{{quartile_line}}":                buildQuartileLine("Q1"),
		"{{kku_report_year}}":              "2568",
		"{{signature}}":                    "ทดสอบ ระบบ",
		"{{document_line}}":                "Full Reprint (บทความตีพิมพ์) จำนวน 1 เรื่อง",
		"{{end_of_contract}}":              "",
		"{{reward_amount}}":                formatAmount(reward),
		"{{manuscript_amount}}":            formatAmount(manuscript),
		"{{page_charge_amount}}":           formatAmount(pageCharge),
		"{{external_fund_list}}":           externalList,
		"{{external_fund_block}}":          buildExternalFundBlock(externalList),
		"{{external_fund_total_negative}}": formatAmountParen(externalTotal),
		"{{net_topup_amount}}":             formatAmount(pageCharge + manuscript - externalTotal),
		"{{reward_received_note}}":         buildRewardReceivedNote(false),
	}
}

func documentXMLFromDocx(t *testing.T, docxPath string) string {
	t.Helper()
	r, err := zip.OpenReader(docxPath)
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open entry: %v", err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			return string(data)
		}
	}
	t.Fatal("word/document.xml not found")
	return ""
}

func fillNewTemplate(t *testing.T, repl map[string]string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := fillDocxTemplate(newTemplateForTest, out, repl); err != nil {
		t.Fatalf("fillDocxTemplate: %v", err)
	}
	return documentXMLFromDocx(t, out)
}

// Every placeholder in the summary table must be substituted; a leftover {{...}} means
// Word split a placeholder across runs in a way normalizeDocxPlaceholders can't repair.
func TestNewTemplate_NoLeftoverPlaceholders(t *testing.T) {
	xml := fillNewTemplate(t, publicationRewardSampleReplacements(45000, 0, 73017.72, []PublicationRewardPreviewExternal{
		{FundName: "กองทุนวิจัย ABC", Amount: "50000"},
	}))
	if leftover := placeholderPattern.FindAllString(xml, -1); len(leftover) > 0 {
		t.Errorf("unsubstituted placeholders remain: %v", leftover)
	}
}

// The summary must foot: reward + A + B - C, with C shown in accounting parentheses.
func TestNewTemplate_SummaryValues(t *testing.T) {
	xml := fillNewTemplate(t, publicationRewardSampleReplacements(45000, 0, 73017.72, []PublicationRewardPreviewExternal{
		{FundName: "กองทุนวิจัย ก", Amount: "20000"},
		{FundName: "บริษัท ข", Amount: "18000"},
		{FundName: "มูลนิธิ ค", Amount: "12000"},
	}))
	for _, want := range []string{
		"45,000.00",   // reward
		"73,017.72",   // page charge (B)
		"(50,000.00)", // external total (C), parenthesised
		"23,017.72",   // net top-up A+B-C
		"68,017.72",   // grand total
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %q in rendered document", want)
		}
	}
}

// When the applicant already received the reward, the reward is 0.00 and the label
// carries the "เคยขอเงินรางวัลแล้ว" note so the zero is explained; otherwise no note.
func TestNewTemplate_RewardReceivedNote(t *testing.T) {
	received := publicationRewardSampleReplacements(0, 0, 73017.72, nil)
	received["{{reward_received_note}}"] = buildRewardReceivedNote(true)
	xml := fillNewTemplate(t, received)
	if !strings.Contains(xml, "เคยขอเงินรางวัลแล้ว") {
		t.Errorf("expected note 'เคยขอเงินรางวัลแล้ว' when reward already received")
	}
	if !strings.Contains(xml, "0.00") {
		t.Errorf("expected reward 0.00 in the already-received case")
	}

	normal := publicationRewardSampleReplacements(45000, 0, 73017.72, nil)
	if strings.Contains(fillNewTemplate(t, normal), "เคยขอเงินรางวัลแล้ว") {
		t.Errorf("note must be absent when reward not previously received")
	}
}

// No external funding: C is (0.00) and the net top-up equals A + B.
func TestNewTemplate_NoExternalFunding(t *testing.T) {
	xml := fillNewTemplate(t, publicationRewardSampleReplacements(45000, 0, 73017.72, nil))
	if !strings.Contains(xml, "(0.00)") {
		t.Errorf("expected external total (0.00) when there is no external funding")
	}
	if !strings.Contains(xml, "73,017.72") {
		t.Errorf("expected net top-up to equal A + B when C = 0")
	}
}

// Guards the deliberate design decision: net top-up mirrors the web's raw
// "เงินสมทบ (A + B - C)" row and is NOT clamped, so it can render negative when
// external funding exceeds the fees.
func TestNewTemplate_NetTopupCanBeNegative(t *testing.T) {
	xml := fillNewTemplate(t, publicationRewardSampleReplacements(45000, 0, 73017.72, []PublicationRewardPreviewExternal{
		{FundName: "แหล่งทุนใหญ่", Amount: "100000"},
	}))
	if !strings.Contains(xml, "(100,000.00)") {
		t.Errorf("expected external total (100,000.00)")
	}
	// 73,017.72 + 0 - 100,000 = -26,982.28
	if !strings.Contains(xml, "-26,982.28") {
		t.Errorf("expected negative net top-up -26,982.28 (unclamped)")
	}
}
