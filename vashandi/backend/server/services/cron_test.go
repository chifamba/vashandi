package services

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseCron
// ---------------------------------------------------------------------------

func TestParseCron_Simple(t *testing.T) {
	cron, err := parseCron("0 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cron.minutes) != 1 || cron.minutes[0] != 0 {
		t.Errorf("minutes: want [0], got %v", cron.minutes)
	}
	if len(cron.hours) != 24 {
		t.Errorf("hours: want 24 values, got %d", len(cron.hours))
	}
}

func TestParseCron_StepSyntax(t *testing.T) {
	cron, err := parseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{0, 15, 30, 45}
	if len(cron.minutes) != len(want) {
		t.Fatalf("minutes: want %v, got %v", want, cron.minutes)
	}
	for i, v := range want {
		if cron.minutes[i] != v {
			t.Errorf("minutes[%d]: want %d, got %d", i, v, cron.minutes[i])
		}
	}
}

func TestParseCron_RangeSyntax(t *testing.T) {
	cron, err := parseCron("0 9-17 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// hours should be 9,10,...,17 — 9 values
	if len(cron.hours) != 9 {
		t.Errorf("hours: want 9 values, got %v", cron.hours)
	}
	if cron.hours[0] != 9 || cron.hours[8] != 17 {
		t.Errorf("hours: want 9..17, got %v", cron.hours)
	}
}

func TestParseCron_CommaSyntax(t *testing.T) {
	cron, err := parseCron("0 0 1,15 * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cron.daysOfMonth) != 2 || cron.daysOfMonth[0] != 1 || cron.daysOfMonth[1] != 15 {
		t.Errorf("daysOfMonth: want [1,15], got %v", cron.daysOfMonth)
	}
}

func TestParseCron_InvalidFieldCount(t *testing.T) {
	_, err := parseCron("* * * *")
	if err == nil {
		t.Error("expected error for 4-field expression")
	}
}

func TestParseCron_OutOfRange(t *testing.T) {
	_, err := parseCron("60 * * * *") // minute 60 is invalid
	if err == nil {
		t.Error("expected error for minute=60")
	}
}

func TestParseCron_EmptyExpression(t *testing.T) {
	_, err := parseCron("")
	if err == nil {
		t.Error("expected error for empty expression")
	}
}

func TestParseCron_InvalidStep(t *testing.T) {
	_, err := parseCron("*/0 * * * *") // step 0 is invalid
	if err == nil {
		t.Error("expected error for step=0")
	}
}

func TestParseCron_RangeStartGTEnd(t *testing.T) {
	_, err := parseCron("0 17-9 * * *")
	if err == nil {
		t.Error("expected error for reversed range 17-9")
	}
}

// ---------------------------------------------------------------------------
// validateCron
// ---------------------------------------------------------------------------

func TestValidateCron_Valid(t *testing.T) {
	if msg := validateCron("0 8 * * 1-5"); msg != "" {
		t.Errorf("expected no error, got %q", msg)
	}
}

func TestValidateCron_Invalid(t *testing.T) {
	if msg := validateCron("not a cron"); msg == "" {
		t.Error("expected an error message for invalid expression")
	}
}

// ---------------------------------------------------------------------------
// nextCronTickInTimezone
// ---------------------------------------------------------------------------

func mustParseUTC(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestNextCronTick_EveryHourAtMinuteZero(t *testing.T) {
	// "0 * * * *" — next tick after 2024-01-01 12:05 UTC should be 13:00
	after := mustParseUTC("2024-01-01 12:05")
	next, err := nextCronTickInTimezone("0 * * * *", "UTC", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next == nil {
		t.Fatal("expected a time, got nil")
	}
	want := mustParseUTC("2024-01-01 13:00")
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, *next)
	}
}

func TestNextCronTick_ExactMinuteAdvancesOneHour(t *testing.T) {
	// After 12:00 exactly, next "0 * * * *" should be 13:00 (strictly after)
	after := mustParseUTC("2024-01-01 12:00")
	next, err := nextCronTickInTimezone("0 * * * *", "UTC", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next == nil {
		t.Fatal("expected a time, got nil")
	}
	want := mustParseUTC("2024-01-01 13:00")
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, *next)
	}
}

func TestNextCronTick_DailyAt8AM(t *testing.T) {
	// "0 8 * * *" — after 2024-01-01 09:00 UTC should be 2024-01-02 08:00 UTC
	after := mustParseUTC("2024-01-01 09:00")
	next, err := nextCronTickInTimezone("0 8 * * *", "UTC", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next == nil {
		t.Fatal("expected a time, got nil")
	}
	want := mustParseUTC("2024-01-02 08:00")
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, *next)
	}
}

func TestNextCronTick_WithTimezone(t *testing.T) {
	// "0 8 * * *" in "America/New_York" (UTC-5 in January)
	// After 2024-01-01 12:00 UTC (07:00 ET), next should be 2024-01-01 13:00 UTC (08:00 ET)
	after := mustParseUTC("2024-01-01 12:00")
	next, err := nextCronTickInTimezone("0 8 * * *", "America/New_York", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next == nil {
		t.Fatal("expected a time, got nil")
	}
	want := mustParseUTC("2024-01-01 13:00")
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, *next)
	}
}

func TestNextCronTick_MonthlyFirstDay(t *testing.T) {
	// "0 0 1 * *" — midnight on the 1st of every month
	// After 2024-01-15 00:00, next should be 2024-02-01 00:00 UTC
	after := mustParseUTC("2024-01-15 00:00")
	next, err := nextCronTickInTimezone("0 0 1 * *", "UTC", after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next == nil {
		t.Fatal("expected a time, got nil")
	}
	want := mustParseUTC("2024-02-01 00:00")
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, *next)
	}
}

func TestNextCronTick_InvalidTimezone(t *testing.T) {
	_, err := nextCronTickInTimezone("* * * * *", "Not/ATimezone", time.Now())
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

func TestNextCronTick_InvalidExpression(t *testing.T) {
	_, err := nextCronTickInTimezone("invalid", "UTC", time.Now())
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}
