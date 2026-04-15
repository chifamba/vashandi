// Package services provides the cron expression parser and next-run calculator
// used by the routine scheduler. It mirrors the behaviour of server/src/services/cron.ts.
//
// Supported 5-field cron syntax (minute hour dom month dow):
//   - `*`       — any value
//   - `N`       — exact value
//   - `N-M`     — inclusive range
//   - `N/S`     — start at N, step S (within field bounds)
//   - `*/S`     — every S from field minimum
//   - `N-M/S`   — range with step
//   - `N,M,...` — comma-separated list of any of the above
package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	// Embed the IANA timezone database so the binary is self-contained.
	_ "time/tzdata"
)

// fieldSpec describes the valid bounds for a single cron field.
type fieldSpec struct {
	min  int
	max  int
	name string
}

var cronFieldSpecs = [5]fieldSpec{
	{0, 59, "minute"},
	{0, 23, "hour"},
	{1, 31, "day of month"},
	{1, 12, "month"},
	{0, 6, "day of week"},
}

// parsedCron holds the expanded, sorted set of valid values for each field.
type parsedCron struct {
	minutes    []int
	hours      []int
	daysOfMonth []int
	months     []int
	daysOfWeek []int
}

// parseCronField expands a single token (e.g. "*/5", "1-3", "0,15,30") into
// the matching integer values within the given field bounds.
func parseCronField(token string, spec fieldSpec) ([]int, error) {
	seen := make(map[int]struct{})

	for _, part := range strings.Split(token, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty element in cron %s field", spec.name)
		}

		// Step syntax: "X/S"
		if idx := strings.Index(part, "/"); idx != -1 {
			base := part[:idx]
			stepStr := part[idx+1:]
			step, err := strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step %q in cron %s field", stepStr, spec.name)
			}

			rangeStart, rangeEnd := spec.min, spec.max
			if base == "*" {
				// */S — every S from min
			} else if dashIdx := strings.Index(base, "-"); dashIdx != -1 {
				// N-M/S
				parts := strings.SplitN(base, "-", 2)
				a, err1 := strconv.Atoi(parts[0])
				b, err2 := strconv.Atoi(parts[1])
				if err1 != nil || err2 != nil {
					return nil, fmt.Errorf("invalid range %q in cron %s field", base, spec.name)
				}
				if err := validateCronBounds(a, spec); err != nil {
					return nil, err
				}
				if err := validateCronBounds(b, spec); err != nil {
					return nil, err
				}
				rangeStart, rangeEnd = a, b
			} else {
				// N/S — start at N
				start, err := strconv.Atoi(base)
				if err != nil {
					return nil, fmt.Errorf("invalid start %q in cron %s field", base, spec.name)
				}
				if err := validateCronBounds(start, spec); err != nil {
					return nil, err
				}
				rangeStart = start
			}

			for i := rangeStart; i <= rangeEnd; i += step {
				seen[i] = struct{}{}
			}
			continue
		}

		// Range syntax: "N-M"
		if dashIdx := strings.Index(part, "-"); dashIdx != -1 {
			parts := strings.SplitN(part, "-", 2)
			a, err1 := strconv.Atoi(parts[0])
			b, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range %q in cron %s field", part, spec.name)
			}
			if err := validateCronBounds(a, spec); err != nil {
				return nil, err
			}
			if err := validateCronBounds(b, spec); err != nil {
				return nil, err
			}
			if a > b {
				return nil, fmt.Errorf("invalid range %d-%d in cron %s field (start > end)", a, b, spec.name)
			}
			for i := a; i <= b; i++ {
				seen[i] = struct{}{}
			}
			continue
		}

		// Wildcard
		if part == "*" {
			for i := spec.min; i <= spec.max; i++ {
				seen[i] = struct{}{}
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q in cron %s field", part, spec.name)
		}
		if err := validateCronBounds(val, spec); err != nil {
			return nil, err
		}
		seen[val] = struct{}{}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("empty result for cron %s field", spec.name)
	}

	result := make([]int, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sort.Ints(result)
	return result, nil
}

func validateCronBounds(v int, spec fieldSpec) error {
	if v < spec.min || v > spec.max {
		return fmt.Errorf("value %d out of range [%d–%d] for cron %s field", v, spec.min, spec.max, spec.name)
	}
	return nil
}

// parseCron parses a 5-field cron expression string and returns the expanded
// value sets for each field.
func parseCron(expression string) (*parsedCron, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, fmt.Errorf("cron expression must not be empty")
	}

	tokens := strings.Fields(trimmed)
	if len(tokens) != 5 {
		return nil, fmt.Errorf("cron expression must have exactly 5 fields, got %d: %q", len(tokens), trimmed)
	}

	minutes, err := parseCronField(tokens[0], cronFieldSpecs[0])
	if err != nil {
		return nil, err
	}
	hours, err := parseCronField(tokens[1], cronFieldSpecs[1])
	if err != nil {
		return nil, err
	}
	dom, err := parseCronField(tokens[2], cronFieldSpecs[2])
	if err != nil {
		return nil, err
	}
	months, err := parseCronField(tokens[3], cronFieldSpecs[3])
	if err != nil {
		return nil, err
	}
	dow, err := parseCronField(tokens[4], cronFieldSpecs[4])
	if err != nil {
		return nil, err
	}

	return &parsedCron{
		minutes:    minutes,
		hours:      hours,
		daysOfMonth: dom,
		months:     months,
		daysOfWeek: dow,
	}, nil
}

// validateCron returns an error message if expression is invalid, or "" if valid.
func validateCron(expression string) string {
	_, err := parseCron(expression)
	if err != nil {
		return err.Error()
	}
	return ""
}

// contains reports whether sorted slice s contains v.
func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// nextCronTickInTimezone calculates the first minute strictly after `after`
// that matches the cron expression when evaluated in the given IANA timezone.
// Returns nil when no match is found within a 4-year search window or when the
// timezone / expression is invalid.
func nextCronTickInTimezone(expression, timezone string, after time.Time) (*time.Time, error) {
	cron, err := parseCron(expression)
	if err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}

	// Advance to the next whole minute (strictly after `after`).
	cursor := after.UTC().Truncate(time.Minute).Add(time.Minute)

	// Safety: search up to 4 years worth of minutes.
	const maxIterations = 4 * 366 * 24 * 60

	for i := 0; i < maxIterations; i++ {
		local := cursor.In(loc)

		month := int(local.Month())     // 1–12
		dom := local.Day()              // 1–31
		dow := int(local.Weekday())     // 0–6 (Sunday=0)
		hour := local.Hour()            // 0–23
		minute := local.Minute()        // 0–59

		if !contains(cron.months, month) {
			// Skip to the first day of the next matching month.
			cursor = advanceToNextMatchingMonth(cursor, loc, cron.months)
			continue
		}

		if !contains(cron.daysOfMonth, dom) || !contains(cron.daysOfWeek, dow) {
			// Advance one day (local midnight).
			cursor = localMidnightPlusDays(cursor, loc, 1)
			continue
		}

		if !contains(cron.hours, hour) {
			next := findNextValue(cron.hours, hour)
			if next >= 0 {
				cursor = localStartOfHour(cursor, loc, next)
			} else {
				// No matching hour left today — advance to tomorrow.
				cursor = localMidnightPlusDays(cursor, loc, 1)
			}
			continue
		}

		if !contains(cron.minutes, minute) {
			next := findNextValue(cron.minutes, minute)
			if next >= 0 {
				cursor = cursor.Truncate(time.Hour).Add(time.Duration(next) * time.Minute)
			} else {
				// No matching minute left this hour — advance to the next hour.
				cursor = cursor.Truncate(time.Hour).Add(time.Hour)
			}
			continue
		}

		// All fields match.
		result := cursor
		return &result, nil
	}

	return nil, nil
}

// findNextValue returns the first value in sorted slice s that is strictly
// greater than current, or -1 if none exists.
func findNextValue(s []int, current int) int {
	for _, v := range s {
		if v > current {
			return v
		}
	}
	return -1
}

// advanceToNextMatchingMonth advances cursor to local midnight on the 1st of
// the next month (in loc) whose 1-based number is in months.
func advanceToNextMatchingMonth(cursor time.Time, loc *time.Location, months []int) time.Time {
	local := cursor.In(loc)
	year, month := local.Year(), int(local.Month())

	for i := 0; i < 48; i++ {
		month++
		if month > 12 {
			month = 1
			year++
		}
		if contains(months, month) {
			t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
			return t.UTC()
		}
	}
	// Should not happen for any valid cron.
	return cursor.Add(366 * 24 * time.Hour)
}

// localMidnightPlusDays returns the UTC instant representing local midnight
// n days after the day of cursor (in loc).
func localMidnightPlusDays(cursor time.Time, loc *time.Location, n int) time.Time {
	local := cursor.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day()+n, 0, 0, 0, 0, loc)
	return next.UTC()
}

// localStartOfHour returns the UTC instant for the start of the given local
// hour on the same day as cursor.
func localStartOfHour(cursor time.Time, loc *time.Location, hour int) time.Time {
	local := cursor.In(loc)
	t := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, loc)
	return t.UTC()
}
