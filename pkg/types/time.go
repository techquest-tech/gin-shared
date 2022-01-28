package types

import "time"

type DateTime time.Time

// const (
var DatetimeFormat = "2006-01-02 15:04:05"

// )

func (t *DateTime) UnmarshalJSON(data []byte) (err error) {
	now, err := time.ParseInLocation(`"`+DatetimeFormat+`"`, string(data), time.Local)
	*t = DateTime(now)
	return
}

func (t DateTime) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(DatetimeFormat)+2)
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, DatetimeFormat)
	b = append(b, '"')
	return b, nil
}

func (t DateTime) String() string {
	return time.Time(t).Format(DatetimeFormat)
}
func (t DateTime) Time() time.Time {
	return time.Time(t)
}
