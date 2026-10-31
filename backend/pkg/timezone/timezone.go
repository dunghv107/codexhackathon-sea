package timezone

import "time"

var VietnamLocation *time.Location

func init() {
	var err error
	VietnamLocation, err = time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		VietnamLocation = time.FixedZone("UTC+7", 7*60*60)
	}
}

// NowVN returns current time in Vietnam timezone
func NowVN() time.Time {
	return time.Now().In(VietnamLocation)
}

// TodayVN returns today's date at 00:00:00 in Vietnam timezone
func TodayVN() time.Time {
	return ToDateVN(time.Now())
}

// ToDateVN converts time to date (00:00:00) in Vietnam timezone
func ToDateVN(t time.Time) time.Time {
	t = t.In(VietnamLocation)
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, VietnamLocation)
}

// ToDateTimeVN converts time to Vietnam timezone
func ToDateTimeVN(t time.Time) time.Time {
	return t.In(VietnamLocation)
}

// ToTimeVN extracts time component only
func ToTimeVN(t time.Time) time.Time {
	h, m, s := t.Clock()
	return time.Date(2000, 1, 1, h, m, s, t.Nanosecond(), VietnamLocation)
}

// IsSameDayVN checks if two times are on the same day in Vietnam timezone
func IsSameDayVN(a, b time.Time) bool {
	a = ToDateVN(a)
	b = ToDateVN(b)
	return a.Equal(b)
}

// StartOfDayVN returns start of day (00:00:00)
func StartOfDayVN(t time.Time) time.Time {
	return ToDateVN(t)
}

// EndOfDayVN returns end of day (23:59:59.999999999)
func EndOfDayVN(t time.Time) time.Time {
	t = t.In(VietnamLocation)
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, 999999999, VietnamLocation)
}

// StartOfMonthVN returns first day of month at 00:00:00
func StartOfMonthVN(t time.Time) time.Time {
	t = t.In(VietnamLocation)
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, VietnamLocation)
}

// EndOfMonthVN returns last moment of month
func EndOfMonthVN(t time.Time) time.Time {
	t = t.In(VietnamLocation)
	y, m, _ := t.Date()
	nextMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, VietnamLocation)
	return nextMonth.Add(-time.Nanosecond)
}

// IsTodayVN checks if time is today in Vietnam timezone
func IsTodayVN(t time.Time) bool {
	return IsSameDayVN(t, NowVN())
}

// GetMonthRange returns start and end of a month
func GetMonthRange(year, month int) (time.Time, time.Time) {
	if month < 1 || month > 12 {
		now := NowVN()
		month = int(now.Month())
		year = now.Year()
	}

	if year <= 0 {
		year = NowVN().Year()
	}

	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, VietnamLocation)
	endDate := EndOfMonthVN(startDate)
	return startDate, endDate
}

// GetWeekRange returns start (Monday) and end (Sunday) of an ISO week
func GetWeekRange(year, week int) (time.Time, time.Time) {
	if week < 1 || week > 53 {
		now := NowVN()
		y, w := now.ISOWeek()
		year = y
		week = w
	}

	date := time.Date(year, time.January, 1, 0, 0, 0, 0, VietnamLocation)

	for date.Weekday() != time.Monday {
		date = date.AddDate(0, 0, -1)
	}

	for {
		y, w := date.ISOWeek()
		if y == year && w == week {
			start := date
			end := start.AddDate(0, 0, 7).Add(-time.Nanosecond)
			return start, end
		}
		date = date.AddDate(0, 0, 7)
	}
}
