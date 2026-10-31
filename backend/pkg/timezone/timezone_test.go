package timezone

import (
	"testing"
	"time"
)

func TestCalendarBoundaries(t *testing.T) {
	utc := time.Date(2024, 2, 29, 18, 30, 4, 5, time.UTC)
	vn := ToDateTimeVN(utc)
	if vn.Day() != 1 || vn.Month() != time.March || vn.Hour() != 1 {
		t.Fatal(vn)
	}
	start := StartOfDayVN(utc)
	end := EndOfDayVN(utc)
	if start.Hour() != 0 || end.Hour() != 23 || end.Nanosecond() != 999999999 || end.Sub(start) != 24*time.Hour-time.Nanosecond {
		t.Fatal(start, end)
	}
	if !IsSameDayVN(utc, start) || IsSameDayVN(start, start.Add(-time.Second)) {
		t.Fatal("day conversion")
	}
	clock := ToTimeVN(vn)
	if clock.Year() != 2000 || clock.Hour() != vn.Hour() || clock.Nanosecond() != 5 {
		t.Fatal(clock)
	}
	a, b := GetMonthRange(2024, 2)
	if a.Day() != 1 || b.Day() != 29 || StartOfMonthVN(b) != a {
		t.Fatal(a, b)
	}
	a, b = GetMonthRange(0, 2)
	if a.Year() != NowVN().Year() || b.Month() != time.February {
		t.Fatal(a, b)
	}
	a, b = GetMonthRange(2024, 0)
	if a.Month() != NowVN().Month() || b.Month() != a.Month() {
		t.Fatal(a, b)
	}
	a, b = GetWeekRange(2020, 53)
	if a.Format("2006-01-02") != "2020-12-28" || b.Format("2006-01-02") != "2021-01-03" {
		t.Fatal(a, b)
	}
	a, b = GetWeekRange(0, 0)
	y, w := NowVN().ISOWeek()
	ay, aw := a.ISOWeek()
	if ay != y || aw != w || b.Sub(a) != 7*24*time.Hour-time.Nanosecond {
		t.Fatal(a, b)
	}
	if !IsTodayVN(NowVN()) || TodayVN().Hour() != 0 {
		t.Fatal("today")
	}
}
