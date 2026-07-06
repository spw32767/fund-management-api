package services

import (
	"context"
	"fmt"
	"fund-management-api/models"
	"sort" // นำเข้าสำหรับใช้เรียงลำดับข้อมูล
	"strings"
	"time"

	"gorm.io/gorm"
)

type ResearchService interface {
	GetResearchByUserID(ctx context.Context, userID uint) ([]models.ResearchDocument, error)
}

type researchService struct {
	db *gorm.DB
}

func NewResearchService(db *gorm.DB) ResearchService {
	return &researchService{db: db}
}

// ==========================================
// Helper Functions
// ==========================================
func toSentenceCase(s string) string {
    s = strings.TrimSpace(s)
    if s == "" {
        return ""
    }
    
    // ตรวจสอบว่าเป็น ALL CAPS หรือไม่ ถ้าใช่ค่อยแปลง (ป้องกันตัวพิมพ์ใหญ่แบบผสมพัง)
    if s == strings.ToUpper(s) {
        s = strings.ToLower(s)
    } else {
        // หากไม่ได้เป็น ALL CAPS ทั้งหมด แต่ต้องการจัดให้อยู่ในมาตรฐานสากล
        s = strings.ToLower(s)
    }
    
    runes := []rune(s)
    // เปลี่ยนอักษรตัวแรกสุดของข้อความเป็นตัวพิมพ์ใหญ่
    if len(runes) > 0 {
        runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
    }
    return string(runes)
}

func getString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func getInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// ==========================================
// Main Service
// ==========================================

func (s *researchService) GetResearchByUserID(
	ctx context.Context,
	userID uint,
) ([]models.ResearchDocument, error) {

	var combinedDocs []models.ResearchDocument

	//currentYear := time.Now().Year()
	//fiveYearsAgo := currentYear - 4 // ครอบคลุม 5 ปีล่าสุด (นับรวมปีปัจจุบัน)

	// ==========================================
	// Ranking Sources
	// ==========================================

	type RankingSource struct {
		SourceID   int    `gorm:"column:source_id"`
		SourceCode string `gorm:"column:source_code"`
	}

	var sources []RankingSource

	s.db.WithContext(ctx).
		Table("ranking_sources").
		Where("is_active = ?", true).
		Find(&sources)

	sourceIDMap := map[string]int{}

	for _, src := range sources {
		sourceIDMap[strings.ToLower(src.SourceCode)] = src.SourceID
	}

	// ==========================================
	// Ranking Weights
	// ==========================================

	var allWeights []models.RankingTierWeight

	s.db.WithContext(ctx).
		Where("is_active = ?", true).
		Find(&allWeights)

	matchWeight := func(
		sourceID int,
		tierCode string,
	) *models.RankingTierWeight {

		for i, w := range allWeights {

			if w.SourceID == sourceID &&
				strings.EqualFold(w.TierCode, tierCode) {

				return &allWeights[i]
			}
		}

		return nil
	}

	// ==========================================
	// Get Target User
	// ==========================================

	type DBUser struct {
		UserID    uint    `gorm:"column:user_id"`
		UserFname string  `gorm:"column:user_fname"`
		UserLname string  `gorm:"column:user_lname"`
		NameEn    string  `gorm:"column:Name_en"`
		ScopusID  *string `gorm:"column:scopus_id"`
	}

	var targetUser DBUser

	s.db.WithContext(ctx).
		Table("users").
		Where("user_id = ?", userID).
		First(&targetUser)

	targetScopusID := ""
	if targetUser.ScopusID != nil {
		targetScopusID = strings.TrimSpace(*targetUser.ScopusID)
	}

	targetLnameEN := ""
	fullNameEN := strings.TrimSpace(targetUser.NameEn)
	if fullNameEN != "" {
		parts := strings.Fields(fullNameEN)
		if len(parts) > 1 {
			targetLnameEN = strings.ToLower(parts[len(parts)-1]) // ดึงคำสุดท้ายชัวร์ว่าเป็นนามสกุลภาษาอังกฤษ
		}
	}

	targetFnameTH := strings.ToLower(strings.TrimSpace(targetUser.UserFname))
	targetLnameTH := strings.ToLower(strings.TrimSpace(targetUser.UserLname))

	// ==========================================
	// SCOPUS
	// ==========================================

	var scopusDocs []models.ScopusDocument

	err := s.db.WithContext(ctx).
		Table("scopus_documents").
		Select("scopus_documents.*, scopus_documents.cover_display_date"). 
    Distinct().
    Joins(`
        JOIN scopus_document_authors
        ON scopus_document_authors.document_id = scopus_documents.id
    `).
		Joins(`
            JOIN scopus_authors
            ON scopus_authors.id = scopus_document_authors.author_id
        `).
		Joins(`
    JOIN users 
    ON users.scopus_id = REPLACE(scopus_authors.scopus_author_id, 'SCOPUS_ID:', '')
`).
		Where("users.user_id = ?", userID).
    Find(&scopusDocs).Error

	if err == nil {
		scopusSourceID := sourceIDMap["scopus"]

		for _, row := range scopusDocs {
			type ScopusAuthor struct {
				FullName       string  `gorm:"column:full_name"`
				ScopusAuthorID *string `gorm:"column:scopus_author_id"`
			}

			var authors []ScopusAuthor

			s.db.WithContext(ctx).
				Table("scopus_document_authors").
				Select(`
                    scopus_authors.full_name,
                    scopus_authors.scopus_author_id
                `).
				Joins(`
                    JOIN scopus_authors
                    ON scopus_authors.id = scopus_document_authors.author_id
                `).
				Where("scopus_document_authors.document_id = ?", row.ID).
				Order("scopus_document_authors.author_seq ASC").
				Scan(&authors)

			var formattedAuthors []string

			for _, a := range authors {
				authorText := strings.TrimSpace(a.FullName)
				isTarget := false

				if a.ScopusAuthorID != nil && targetScopusID != "" && *a.ScopusAuthorID == targetScopusID {
					isTarget = true
				}
				if !isTarget && targetLnameEN != "" && strings.Contains(strings.ToLower(authorText), targetLnameEN) {
					isTarget = true
				}

				authorText = strings.ReplaceAll(authorText, ",", "")
				parts := strings.Fields(authorText)
				if len(parts) >= 2 {
					lastName := parts[0]
					firstNamePart := parts[1]
					initial := strings.ToUpper(string([]rune(firstNamePart)[0]))
					authorText = fmt.Sprintf("%s, %s.", lastName, initial)
				}

				if isTarget {
					authorText = fmt.Sprintf("<strong>%s</strong>", authorText)
				}

				formattedAuthors = append(formattedAuthors, authorText)
			}

			authorsText := ""
			if len(formattedAuthors) > 1 {
				authorsText = strings.Join(formattedAuthors[:len(formattedAuthors)-1], ", ") + ", & " + formattedAuthors[len(formattedAuthors)-1]
			} else if len(formattedAuthors) == 1 {
				authorsText = formattedAuthors[0]
			} else {
				authorsText = "Unknown Author"
			}

			year := 0
			if row.CoverDate != nil {
				year = row.CoverDate.Year()
			}

			tierCode := getString(row.Subtype)

			// map subtype → tier_code จริงใน DB
			scopusTierMap := map[string]string{
				"cp": "scopus_conf",
				"ar": "scopus_journal_q1_q4",
				"re": "scopus_journal_q1_q4",
				"ip": "scopus_journal_q1_q4",
				"sh": "scopus_journal_q1_q4",
				"no": "scopus_journal_q1_q4",
				"ed": "scopus_journal_q1_q4",
				"le": "scopus_journal_q1_q4",
				"bk": "scopus_journal_q1_q4",
				"ch": "scopus_journal_q1_q4",
			}
			if mapped, ok := scopusTierMap[strings.ToLower(tierCode)]; ok {
				tierCode = mapped
			}

			cleanTitle := toSentenceCase(getString(row.Title))

            isConference := false
            if row.AggregationType != nil && *row.AggregationType == "Conference Proceeding" {
                isConference = true
            } else if row.Subtype != nil && *row.Subtype == "cp" {
                isConference = true
            }

            // --- แก้ไขปัญหาข้อมูลการประชุมว่างเปล่า ---
            confName := getString(row.ConferenceName)
            if isConference && confName == "" {
                // ถ้าเป็นงานประชุมแต่ไม่มีชื่อ conference แยกมา ให้ยืมชื่อเล่มสิ่งตีพิมพ์ (PublicationName) มาใช้แทน
                confName = getString(row.PublicationName)
            }

            doc := models.ResearchDocument{
                ID:           row.ID,
                UserID:       userID,
                SourceType:   "scopus",
                Authors:      authorsText,
                Title:        cleanTitle, // ใช้ตัวที่แปลงเคสแล้ว
                JournalName:  getString(row.PublicationName),
                PublishYear:  year,
                Volume:       getString(row.Volume),
                Issue:        getString(row.Issue),
                Pages:        getString(row.PageRange),
                DOI:          getString(row.DOI),
                ArticleURL:   getString(row.ScopusLink),
                IsConference: isConference,

                // Conference fields
                ConferenceName:   confName, // ผ่านการตรวจสอบดักค่าว่างเรียบร้อย
                City:             getString(row.ConferenceCity),
                Country:          getString(row.ConferenceCountry),
                CoverDisplayDate: getString(row.CoverDisplayDate),
                ConferenceVenue:  row.ConferenceVenue,

                ConferenceDateStart: nil, // 
ConferenceDateEnd:   nil,

                UpdatedAt: func() time.Time {
                    if row.CoverDate != nil {
                        return row.CoverDate.Local()
                    }
                    return time.Time{}
                }(),
            }
			doc.TierDetails = matchWeight(scopusSourceID, tierCode)

			fmt.Printf("[DEBUG] source=scopus tierCode=%q sourceID=%d weight=%v\n",
				tierCode, scopusSourceID, matchWeight(scopusSourceID, tierCode))

			for _, w := range allWeights {
				fmt.Printf("[DEBUG] weight: source_id=%d tier_code=%q tier_name=%q\n",
					w.SourceID, w.TierCode, w.TierName)
			}
			combinedDocs = append(combinedDocs, doc)
		}
	} else {
		fmt.Println("SCOPUS QUERY ERROR:", err)
	}

	// ==========================================
	// THAIJO
	// ==========================================

	var thaijoDocs []models.ThaiJODocument

	err = s.db.WithContext(ctx).
		Table("thaijo_documents").
		Distinct("thaijo_documents.*").
		Joins(`
            JOIN thaijo_document_authors
            ON thaijo_document_authors.document_id = thaijo_documents.id
        `).
		Joins(`
            JOIN thaijo_authors
            ON thaijo_authors.id = thaijo_document_authors.author_id
        `).
		Joins(`
            JOIN users
            ON CONCAT(users.user_fname, ' ', users.user_lname) = thaijo_authors.full_name_th
        `).
		Where("users.user_id = ?", userID).
    Find(&thaijoDocs).Error

	if err == nil {
		thaijoSourceID := sourceIDMap["tci"]

		for _, row := range thaijoDocs {
			type ThaiJOAuthorSchema struct {
				NameEn *string `gorm:"column:name_en"`
				NameTh *string `gorm:"column:name_th"`
			}

			var authors []ThaiJOAuthorSchema

			s.db.WithContext(ctx).
				Table("thaijo_document_authors").
				Select("name_en, name_th").
				Where("document_id = ?", row.ID).
				Order("author_seq ASC").
				Scan(&authors)

			var formattedAuthors []string

			for _, a := range authors {
				authorText := ""
				isTarget := false

				// 1. เช็คเป้าหมายอาจารย์ผู้ค้นหา
				if a.NameEn != nil && targetLnameEN != "" && strings.Contains(strings.ToLower(*a.NameEn), targetLnameEN) {
					isTarget = true
				}
				if !isTarget && a.NameTh != nil {
					cleanTH := strings.ToLower(*a.NameTh)
					if (targetFnameTH != "" && strings.Contains(cleanTH, targetFnameTH)) ||
						(targetLnameTH != "" && strings.Contains(cleanTH, targetLnameTH)) {
						isTarget = true
					}
				}

				// 2. จัดฟอร์แมตภาษาอังกฤษ
				if a.NameEn != nil && strings.TrimSpace(*a.NameEn) != "" {
					rawNameEN := strings.TrimSpace(*a.NameEn)
					rawNameEN = strings.ReplaceAll(rawNameEN, ",", "")
					parts := strings.Fields(rawNameEN)

					if len(parts) >= 2 {
						var lastName, firstName string

						if isTarget && targetLnameEN != "" {
							if strings.ToLower(parts[0]) == targetLnameEN {
								lastName = parts[0]
								firstName = parts[1]
							} else {
								lastName = parts[len(parts)-1]
								firstName = parts[0]
							}
						} else {
							lastName = parts[0]
							firstName = parts[1]
						}

						if firstName != "" && lastName != "" {
							initial := strings.ToUpper(string([]rune(firstName)[0]))
							authorText = fmt.Sprintf("%s, %s.", lastName, initial)
						}
					} else if len(parts) == 1 {
						authorText = parts[0]
					}
				}

				// 3. Fallback สำหรับรายชื่อที่มีแต่ภาษาไทย
				if authorText == "" && a.NameTh != nil && strings.TrimSpace(*a.NameTh) != "" {
					rawNameTH := strings.TrimSpace(*a.NameTh)
					replacer := strings.NewReplacer(
						"รศ.ดร.", "", "ผศ.ดร.", "", "ดร.", "", "อ.", "",
						"ศาสตราจารย์", "", "รองศาสตราจารย์", "", "ผู้ช่วยศาสตราจารย์", "",
						"นาย", "", "นางสาว", "", "นาง", "",
					)
					cleanNameTH := strings.TrimSpace(replacer.Replace(rawNameTH))
					partsTH := strings.Fields(cleanNameTH)

					if len(partsTH) >= 2 {
						authorText = fmt.Sprintf("%s %s", partsTH[0], partsTH[len(partsTH)-1])
					} else {
						authorText = cleanNameTH
					}
				}

				if isTarget && authorText != "" {
					authorText = fmt.Sprintf("<strong>%s</strong>", authorText)
				}

				if authorText != "" {
					formattedAuthors = append(formattedAuthors, authorText)
				}
			}

			authorsText := ""
			if len(formattedAuthors) > 1 {
				authorsText = strings.Join(formattedAuthors[:len(formattedAuthors)-1], ", ") + ", & " + formattedAuthors[len(formattedAuthors)-1]
			} else if len(formattedAuthors) == 1 {
				authorsText = formattedAuthors[0]
			} else {
				authorsText = "Unknown Author"
			}

			rawTitle := getString(row.TitleEN)
            if rawTitle == "" {
                rawTitle = getString(row.TitleTH)
            }
            cleanTitle := toSentenceCase(rawTitle)

            journalName := getString(row.JournalPath)
            var journalRow models.ThaiJOJournal
            tierCode := ""

            if row.JournalID != nil {
                err := s.db.WithContext(ctx).
                    Where("journal_id = ?", *row.JournalID).
                    First(&journalRow).Error

                if err == nil {
                    if journalRow.NameEN != nil {
                        journalName = getString(journalRow.NameEN)
                    }
                    if journalRow.Tier != nil {
                        thaijoTierMap := map[int]string{
                            1: "tci_t1",
                            2: "tci_t2",
                            3: "tci_conf",
                        }
                        if mapped, ok := thaijoTierMap[*journalRow.Tier]; ok {
                            tierCode = mapped
                        }
                    }
                }
            }

            journalLower := strings.ToLower(journalName)
            isThaiJOConference := strings.Contains(journalLower, "proceedings") ||
                strings.Contains(journalLower, "conference") ||
                strings.Contains(journalLower, "ประชุมวิชาการ")

            doc := models.ResearchDocument{
                ID:           uint(row.ID),
                UserID:       userID,
                SourceType:   "thaijo",
                Authors:      authorsText,
                Title:        cleanTitle, // ส่งชื่อบทความที่ทำความสะอาดตัวพิมพ์ใหญ่ไป
                JournalName:  journalName,
                PublishYear:  getInt(row.Year),
                DOI:          getString(row.DOI),
                ArticleURL:   getString(row.ArticleURL),
                IsConference: isThaiJOConference,

                ConferenceName:      journalName,
                City:                "",
                Country:             "",
                ConferenceDateStart: row.DatePublished,
                ConferenceDateEnd:   nil,

                UpdatedAt: row.UpdatedAt,
            }

			doc.TierDetails = matchWeight(thaijoSourceID, tierCode)
			combinedDocs = append(combinedDocs, doc)
		}
	} else {
		fmt.Println("THAIJO QUERY ERROR:", err)
	}

	// =========================================================================
	// [Sorting Engine] เรียงลำดับข้อมูลผลลัพธ์รวมจากปีปัจจุบันไปอดีต (ใหม่สุด -> เก่าสุด)
	// =========================================================================
	sort.Slice(combinedDocs, func(i, j int) bool {
		return combinedDocs[i].PublishYear > combinedDocs[j].PublishYear
	})

	return combinedDocs, nil
} 