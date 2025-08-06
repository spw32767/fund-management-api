// utils/additional_info.go (Version แบบยืดหยุ่น)
package utils

import (
	"strings"
	"unicode"
)

// SplitCommaDelimited แยกข้อความที่คั่นด้วยจุลภาค (เหมือนเดิม)
func SplitCommaDelimited(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	var cleaned []string

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return cleaned
}

// JoinCommaDelimited รวมข้อความด้วยจุลภาค (เหมือนเดิม)
func JoinCommaDelimited(items []string) string {
	var cleaned []string

	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, ", ")
}

// ValidateFundingReferences - แบบยืดหยุ่น (รองรับรูปแบบใดก็ได้)
func ValidateFundingReferences(input string) (bool, string) {
	if input == "" {
		return true, "" // ไม่บังคับใส่
	}

	refs := SplitCommaDelimited(input)
	for _, ref := range refs {
		// ตรวจสอบความยาวขั้นต่ำ
		if len(ref) < 2 {
			return false, "หมายเลขอ้างอิงทุนต้องมีอย่างน้อย 2 ตัวอักษร"
		}

		// ตรวจสอบความยาวสูงสุด (ป้องกันการใส่ข้อมูลมากเกินไป)
		if len(ref) > 100 {
			return false, "หมายเลขอ้างอิงทุนไม่ควรยาวเกิน 100 ตัวอักษร"
		}

		// ตรวจสอบว่าไม่ใช่ช่องว่างทั้งหมด
		if strings.TrimSpace(ref) == "" {
			return false, "หมายเลขอ้างอิงทุนไม่สามารถเป็นช่องว่างได้"
		}

		// ตรวจสอบว่าไม่มีอักขระควบคุม (control characters)
		for _, char := range ref {
			if unicode.IsControl(char) && char != '\t' {
				return false, "หมายเลขอ้างอิงทุนมีอักขระที่ไม่ถูกต้อง"
			}
		}
	}

	return true, ""
}

// ValidateFundingReferencesStrict - แบบเข้มงวด (สำหรับกรณีที่ต้องการรูปแบบเฉพาะ)
func ValidateFundingReferencesStrict(input string) (bool, string) {
	if input == "" {
		return true, ""
	}

	refs := SplitCommaDelimited(input)
	for _, ref := range refs {
		if len(ref) < 2 {
			return false, "หมายเลขอ้างอิงทุนต้องมีอย่างน้อย 2 ตัวอักษร"
		}

		// ตรวจสอบรูปแบบเข้มงวด (เฉพาะตัวอักษร ตัวเลข เครื่องหมาย - และ _)
		for _, char := range ref {
			if !((char >= 'A' && char <= 'Z') ||
				(char >= 'a' && char <= 'z') ||
				(char >= '0' && char <= '9') ||
				char == '-' || char == '_') {
				return false, "หมายเลขอ้างอิงทุนควรมีเฉพาะตัวอักษร ตัวเลข เครื่องหมาย - และ _"
			}
		}
	}

	return true, ""
}

// ValidateUniversityRankings - ยืดหยุ่น (รองรับข้อความใดก็ได้)
func ValidateUniversityRankings(input string) (bool, string) {
	if input == "" {
		return true, ""
	}

	rankings := SplitCommaDelimited(input)
	for _, ranking := range rankings {
		if len(ranking) < 3 {
			return false, "อันดับมหาวิทยาลัยต้องมีความยาวอย่างน้อย 3 ตัวอักษร"
		}

		if len(ranking) > 200 {
			return false, "อันดับมหาวิทยาลัยไม่ควรยาวเกิน 200 ตัวอักษร"
		}

		// ตรวจสอบว่าไม่มีอักขระควบคุม
		for _, char := range ranking {
			if unicode.IsControl(char) && char != '\t' {
				return false, "อันดับมหาวิทยาลัยมีอักขระที่ไม่ถูกต้อง"
			}
		}
	}

	return true, ""
}
