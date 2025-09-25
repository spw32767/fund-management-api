package utils

import (
	"math"
	"strconv"
	"strings"
	"time"
)

var thaiMonths = []string{
	"มกราคม",
	"กุมภาพันธ์",
	"มีนาคม",
	"เมษายน",
	"พฤษภาคม",
	"มิถุนายน",
	"กรกฎาคม",
	"สิงหาคม",
	"กันยายน",
	"ตุลาคม",
	"พฤศจิกายน",
	"ธันวาคม",
}

// FormatThaiDate returns the date formatted using Thai month names and Buddhist Era year.
func FormatThaiDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	localTime := t.In(time.Local)
	monthIndex := int(localTime.Month()) - 1
	if monthIndex < 0 || monthIndex >= len(thaiMonths) {
		return localTime.Format("02/01/2006")
	}

	day := localTime.Day()
	monthName := thaiMonths[monthIndex]
	year := localTime.Year() + 543

	return strconv.Itoa(day) + " " + monthName + " " + strconv.Itoa(year)
}

// FormatThaiDatePtr returns Thai formatted date for pointer values.
func FormatThaiDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return FormatThaiDate(*t)
}

var thaiDigits = []string{"ศูนย์", "หนึ่ง", "สอง", "สาม", "สี่", "ห้า", "หก", "เจ็ด", "แปด", "เก้า"}
var thaiUnits = []string{"", "สิบ", "ร้อย", "พัน", "หมื่น", "แสน"}

// BahtText converts a decimal amount into its Thai Baht text representation.
func BahtText(amount float64) string {
	if amount == 0 {
		return "ศูนย์บาทถ้วน"
	}

	totalSatang := int64(math.Round(amount * 100))
	baht := totalSatang / 100
	satang := totalSatang % 100

	bahtText := readThaiNumber(baht)
	if baht == 0 {
		bahtText = thaiDigits[0]
	}

	result := bahtText + "บาท"
	if satang == 0 {
		result += "ถ้วน"
		return result
	}

	result += readThaiNumber(satang) + "สตางค์"
	return result
}

func readThaiNumber(number int64) string {
	if number == 0 {
		return ""
	}

	if number >= 1000000 {
		millions := number / 1000000
		remainder := number % 1000000
		builder := strings.Builder{}
		builder.WriteString(readThaiNumber(millions))
		builder.WriteString("ล้าน")
		if remainder > 0 {
			builder.WriteString(readThaiNumber(remainder))
		}
		return builder.String()
	}

	return readThaiNumberWithinMillion(number)
}

func readThaiNumberWithinMillion(number int64) string {
	str := strconv.FormatInt(number, 10)
	length := len(str)
	var parts []string

	for i, ch := range str {
		digit := int(ch - '0')
		position := length - i - 1
		if digit == 0 {
			continue
		}

		switch position {
		case 0:
			if length > 1 && digit == 1 {
				parts = append(parts, "เอ็ด")
			} else {
				parts = append(parts, thaiDigits[digit])
			}
		case 1:
			if digit == 1 {
				parts = append(parts, "สิบ")
			} else if digit == 2 {
				parts = append(parts, "ยี่สิบ")
			} else {
				parts = append(parts, thaiDigits[digit]+thaiUnits[position])
			}
		default:
			parts = append(parts, thaiDigits[digit]+thaiUnits[position])
		}
	}

	return strings.Join(parts, "")
}
