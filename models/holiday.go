package models

import "time"

type Holiday struct {
	Date        time.Time
	Day         string
	Description string
}

// Holidays2026 berisi daftar hari libur nasional Indonesia tahun 2026
var Holidays2026 = []Holiday{
	{mustDate("2026-01-01"), "Kamis", "Tahun Baru 2026 Masehi"},
	{mustDate("2026-01-16"), "Jumat", "Isra Mikraj Nabi Muhammad SAW"},
	{mustDate("2026-02-17"), "Selasa", "Tahun Baru Imlek 2577 Kongzili"},
	{mustDate("2026-03-19"), "Kamis", "Hari Suci Nyepi (Tahun Baru Saka 1948)"},
	{mustDate("2026-03-21"), "Sabtu", "Hari Raya Idul Fitri 1447 Hijriah"},
	{mustDate("2026-03-22"), "Minggu", "Hari Raya Idul Fitri 1447 Hijriah"},
	{mustDate("2026-04-03"), "Jumat", "Wafat Yesus Kristus"},
	{mustDate("2026-04-05"), "Minggu", "Kebangkitan Yesus Kristus (Paskah)"},
	{mustDate("2026-05-01"), "Jumat", "Hari Buruh Internasional"},
	{mustDate("2026-05-14"), "Kamis", "Kenaikan Yesus Kristus"},
	{mustDate("2026-05-27"), "Rabu", "Hari Raya Idul Adha 1447 Hijriah"},
	{mustDate("2026-05-31"), "Minggu", "Hari Raya Waisak 2570 BE"},
	{mustDate("2026-06-01"), "Senin", "Hari Lahir Pancasila"},
	{mustDate("2026-06-16"), "Selasa", "Tahun Baru Islam 1448 Hijriah"},
	{mustDate("2026-08-17"), "Senin", "Hari Kemerdekaan Republik Indonesia"},
	{mustDate("2026-08-25"), "Selasa", "Maulid Nabi Muhammad SAW"},
	{mustDate("2026-12-25"), "Jumat", "Hari Raya Natal (Kelahiran Yesus Kristus)"},
}

// IsHoliday mengecek apakah suatu tanggal adalah hari libur nasional
func IsHoliday(t time.Time) bool {
	d := t.Truncate(24 * time.Hour)
	for _, h := range Holidays2026 {
		if h.Date.Equal(d) {
			return true
		}
	}
	return false
}

// GetHoliday mengembalikan Holiday jika tanggal tersebut libur, nil jika tidak
func GetHoliday(t time.Time) *Holiday {
	d := t.Truncate(24 * time.Hour)
	for i, h := range Holidays2026 {
		if h.Date.Equal(d) {
			return &Holidays2026[i]
		}
	}
	return nil
}

func mustDate(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		panic("invalid holiday date: " + s)
	}
	return t
}
