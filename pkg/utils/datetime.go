package utils

import (
	"fmt"
	"time"
)

var (
	persianWeekdays = [...]string{
		time.Sunday:    "یک‌شنبه",
		time.Monday:    "دوشنبه",
		time.Tuesday:   "سه‌شنبه",
		time.Wednesday: "چهارشنبه",
		time.Thursday:  "پنج‌شنبه",
		time.Friday:    "جمعه",
		time.Saturday:  "شنبه",
	}

	persianMonths = [...]string{
		1:  "فروردین",
		2:  "اردیبهشت",
		3:  "خرداد",
		4:  "تیر",
		5:  "مرداد",
		6:  "شهریور",
		7:  "مهر",
		8:  "آبان",
		9:  "آذر",
		10: "دی",
		11: "بهمن",
		12: "اسفند",
	}
)

type ExtractionTimeInfo struct {
	ShamsiDate      string `json:"shamsi_date"`      // e.g. 1405/08/18
	DayOfWeek       string `json:"day_of_week"`      // e.g. دوشنبه
	ShamsiFullText  string `json:"shamsi_full_text"` // e.g. دوشنبه ۱۸ آبان ۱۴۰۵ ساعت ۱۰:۴۵
	GregorianDate   string `json:"gregorian_date"`   // e.g. 2026-11-09
	Timestamp       string `json:"timestamp"`        // ISO 8601
}

// NowExtractionInfo returns the current date, Persian day of week, and timestamps.
func NowExtractionInfo() ExtractionTimeInfo {
	return FormatExtractionInfo(time.Now())
}

// FormatExtractionInfo converts any time.Time to ExtractionTimeInfo.
func FormatExtractionInfo(t time.Time) ExtractionTimeInfo {
	jy, jm, jd := GregorianToJalali(t.Year(), int(t.Month()), t.Day())
	dayName := persianWeekdays[t.Weekday()]
	monthName := persianMonths[jm]

	shamsiDate := fmt.Sprintf("%04d/%02d/%02d", jy, jm, jd)
	shamsiFullText := fmt.Sprintf("%s %d %s %04d ساعت %02d:%02d:%02d", dayName, jd, monthName, jy, t.Hour(), t.Minute(), t.Second())
	gregorianDate := t.Format("2006-01-02")
	timestamp := t.Format(time.RFC3339)

	return ExtractionTimeInfo{
		ShamsiDate:     shamsiDate,
		DayOfWeek:      dayName,
		ShamsiFullText: shamsiFullText,
		GregorianDate:  gregorianDate,
		Timestamp:      timestamp,
	}
}

// GregorianToJalali converts a Gregorian year, month, day to Jalali (Solar Hijri) year, month, day.
func GregorianToJalali(gy, gm, gd int) (jy, jm, jd int) {
	gDaysInMonth := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	jDaysInMonth := [...]int{31, 31, 31, 31, 31, 31, 30, 30, 30, 30, 30, 29}

	gy -= 1600
	gm -= 1
	gd -= 1

	gDayNo := 365*gy + (gy+3)/4 - (gy+99)/100 + (gy+399)/400
	for i := 0; i < gm; i++ {
		gDayNo += gDaysInMonth[i]
	}
	if gm > 1 && ((gy%4 == 0 && gy%100 != 0) || (gy%400 == 0)) {
		gDayNo++
	}
	gDayNo += gd

	jDayNo := gDayNo - 79
	jNp := jDayNo / 12053
	jDayNo %= 12053

	jy = 979 + 33*jNp + 4*(jDayNo/1461)
	jDayNo %= 1461

	if jDayNo >= 366 {
		jy += (jDayNo - 1) / 365
		jDayNo = (jDayNo - 1) % 365
	}

	for i := 0; i < 11; i++ {
		if jDayNo < jDaysInMonth[i] {
			jm = i + 1
			jd = jDayNo + 1
			return jy, jm, jd
		}
		jDayNo -= jDaysInMonth[i]
	}

	jm = 12
	jd = jDayNo + 1
	return jy, jm, jd
}

