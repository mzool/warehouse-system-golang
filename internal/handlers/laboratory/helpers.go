package laboratory

import (
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// parseFlexibleDate parses date strings in multiple formats and returns pgtype.Date
func parseFlexibleDate(dateStr string) pgtype.Date {
	if dateStr == "" {
		return pgtype.Date{Valid: false}
	}

	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return pgtype.Date{Time: t, Valid: true}
		}
	}

	return pgtype.Date{Valid: false}
}

// parseFlexibleTimestamp parses timestamp strings in multiple formats and returns pgtype.Timestamptz
func parseFlexibleTimestamp(timestampStr string) pgtype.Timestamptz {
	if timestampStr == "" {
		return pgtype.Timestamptz{Valid: false}
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestampStr); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	return pgtype.Timestamptz{Valid: false}
}

// floatToNumeric converts float64 to pgtype.Numeric
// Uses 2 decimal places precision for most measurements
func floatToNumeric(f float64) pgtype.Numeric {
	if f == 0 {
		return pgtype.Numeric{Valid: false}
	}
	return pgtype.Numeric{
		Int:   new(big.Int).SetInt64(int64(f * 100)),
		Exp:   -2,
		Valid: true,
	}
}

// floatToNumericPrecision converts float64 to pgtype.Numeric with custom precision
// precision specifies number of decimal places (e.g., 4 for 0.0001 precision)
func floatToNumericPrecision(f float64, precision int32) pgtype.Numeric {
	if f == 0 {
		return pgtype.Numeric{Valid: false}
	}

	multiplier := new(big.Float).SetInt64(1)
	for i := int32(0); i < precision; i++ {
		multiplier.Mul(multiplier, big.NewFloat(10))
	}

	scaled := new(big.Float).Mul(big.NewFloat(f), multiplier)
	intVal, _ := scaled.Int(nil)

	return pgtype.Numeric{
		Int:   intVal,
		Exp:   -precision,
		Valid: true,
	}
}

// stringToNumeric parses string to pgtype.Numeric using Scan
func stringToNumeric(s string) pgtype.Numeric {
	if s == "" {
		return pgtype.Numeric{Valid: false}
	}

	var num pgtype.Numeric
	if err := num.Scan(s); err != nil {
		return pgtype.Numeric{Valid: false}
	}
	return num
}

// numericToFloat converts pgtype.Numeric to float64
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid || n.Int == nil {
		return 0
	}

	value := float64(n.Int.Int64())
	if n.Exp != 0 {
		for i := int32(0); i < -n.Exp; i++ {
			value /= 10
		}
		for i := int32(0); i < n.Exp; i++ {
			value *= 10
		}
	}
	return value
}

// int32Ptr safely converts int32 to pgtype.Int4 if value > 0
func int32Ptr(val int32) pgtype.Int4 {
	if val > 0 {
		return pgtype.Int4{Int32: val, Valid: true}
	}
	return pgtype.Int4{Valid: false}
}

// textPtr safely converts string to pgtype.Text if not empty
func textPtr(s string) pgtype.Text {
	if s != "" {
		return pgtype.Text{String: s, Valid: true}
	}
	return pgtype.Text{Valid: false}
}

// boolPtr safely converts bool pointer to pgtype.Bool
func boolPtr(b *bool) pgtype.Bool {
	if b != nil {
		return pgtype.Bool{Bool: *b, Valid: true}
	}
	return pgtype.Bool{Valid: false}
}

// currentTimestamp returns current time as pgtype.Timestamptz
func currentTimestamp() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

// currentDate returns current date as pgtype.Date
func currentDate() pgtype.Date {
	return pgtype.Date{Time: time.Now(), Valid: true}
}

// dateOrDefault returns parsed date or default if parsing fails
func dateOrDefault(dateStr string, defaultDate pgtype.Date) pgtype.Date {
	parsed := parseFlexibleDate(dateStr)
	if parsed.Valid {
		return parsed
	}
	return defaultDate
}

// timestampOrDefault returns parsed timestamp or default if parsing fails
func timestampOrDefault(timestampStr string, defaultTime pgtype.Timestamptz) pgtype.Timestamptz {
	parsed := parseFlexibleTimestamp(timestampStr)
	if parsed.Valid {
		return parsed
	}
	return defaultTime
}

// timestampOrNow returns parsed timestamp or current time if parsing fails
func timestampOrNow(timestampStr string) pgtype.Timestamptz {
	return timestampOrDefault(timestampStr, currentTimestamp())
}

// dateFromDaysAgo returns date N days ago from now
func dateFromDaysAgo(days int) pgtype.Date {
	return pgtype.Date{Time: time.Now().AddDate(0, 0, -days), Valid: true}
}

// dateAddDays returns date N days from given date
func dateAddDays(base pgtype.Date, days int) pgtype.Date {
	if !base.Valid {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: base.Time.AddDate(0, 0, days), Valid: true}
}

// formatError creates a user-friendly error message
func formatError(field string, err error) string {
	return fmt.Sprintf("invalid %s: %v", field, err)
}
