package gobash

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() { Register("date", cmdDate) }

var (
	relativeDatePattern = regexp.MustCompile(`^([+-]?)\s*(\d+)\s*(second|minute|hour|day|week)s?(?:\s+(ago))?$`)
	anchoredDatePattern = regexp.MustCompile(`^(.+?)\s+([+-]?\s*\d+\s*(?:second|minute|hour|day|week)s?(?:\s+ago)?)$`)
	unixDatePattern     = regexp.MustCompile(`^@\s*([+-]?\d+)$`)
)

func cmdDate(_ context.Context, e *Env) int {
	now := e.Now()
	utc := false
	format := ""
	dateInput := ""
	for i := 1; i < len(e.Args); i++ {
		arg := e.Args[i]
		switch {
		case arg == "-u" || arg == "--utc" || arg == "--universal":
			utc = true
		case arg == "-d" || arg == "--date":
			if i+1 >= len(e.Args) {
				e.Errorf("option %s requires an argument", arg)
				return 2
			}
			i++
			dateInput = e.Args[i]
		case strings.HasPrefix(arg, "--date="):
			dateInput = strings.TrimPrefix(arg, "--date=")
		case strings.HasPrefix(arg, "+"):
			format = strings.TrimPrefix(arg, "+")
		case arg == "-Iseconds" || arg == "--iso-8601=seconds":
			format = "%Y-%m-%dT%H:%M:%S%:z"
		case arg == "--help":
			_, _ = fmt.Fprintln(e.Stdout, `usage: date [-u] [-d date] [+format]
  -d accepts now, today, tomorrow, yesterday, @TIMESTAMP,
  YYYY-MM-DD, YYYY-MM-DD HH:MM:SS[ UTC| +/-HHMM| +/-HH:MM], RFC3339,
  relative N seconds|minutes|hours|days|weeks [ago], and an absolute
  anchor followed by one relative offset (for example: DATE + 1 day)`)
			return 0
		default:
			e.Errorf("unsupported operand %q", arg)
			return 2
		}
	}
	if dateInput != "" {
		parseBase := now
		if utc {
			// GNU date interprets timezone-free --date operands in UTC when -u is
			// active. Parsing in the host clock's location and converting later
			// shifts midnight (for example, CET midnight becomes 23:00 UTC).
			parseBase = now.UTC()
		}
		parsed, err := parseDateInput(parseBase, dateInput)
		if err != nil {
			e.Errorf("invalid date %q: %v", dateInput, err)
			return 1
		}
		now = parsed
	}
	if utc {
		now = now.UTC()
	}
	output := now.Format("Mon Jan _2 15:04:05 MST 2006")
	if format != "" {
		output = formatDate(now, format)
	}
	if _, err := fmt.Fprintln(e.Stdout, output); err != nil {
		e.Errorf("%v", err)
		return 1
	}
	return 0
}

func parseDateInput(base time.Time, input string) (time.Time, error) {
	input = strings.TrimSpace(input)
	normalized := strings.ToLower(input)
	switch normalized {
	case "now":
		return base, nil
	case "today":
		return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()), nil
	case "tomorrow":
		return base.AddDate(0, 0, 1), nil
	case "yesterday":
		return base.AddDate(0, 0, -1), nil
	}
	if match := unixDatePattern.FindStringSubmatch(normalized); match != nil {
		value, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(value, 0), nil
	}
	if parsed, ok := parseAbsoluteDate(base, input); ok {
		return parsed, nil
	}
	if amount, unit, ok := parseRelativeDate(normalized); ok {
		return applyRelativeDate(base, amount, unit), nil
	}
	if match := anchoredDatePattern.FindStringSubmatch(normalized); match != nil {
		anchor := strings.TrimSpace(input[:len(match[1])])
		parsed, ok := parseAbsoluteDate(base, anchor)
		if !ok {
			return time.Time{}, fmt.Errorf("unsupported anchor %q", anchor)
		}
		amount, unit, ok := parseRelativeDate(match[2])
		if ok {
			return applyRelativeDate(parsed, amount, unit), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported format")
}

func parseAbsoluteDate(base time.Time, input string) (time.Time, bool) {
	if strings.HasSuffix(input, " UTC") {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05 MST", input, time.UTC); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 -07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.ParseInLocation(layout, input, base.Location()); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseRelativeDate(input string) (int, string, bool) {
	match := relativeDatePattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(input)))
	if match == nil {
		return 0, "", false
	}
	amount, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, "", false
	}
	if match[1] == "-" || match[4] == "ago" {
		amount = -amount
	}
	return amount, match[3], true
}

func applyRelativeDate(base time.Time, amount int, unit string) time.Time {
	switch unit {
	case "second":
		return base.Add(time.Duration(amount) * time.Second)
	case "minute":
		return base.Add(time.Duration(amount) * time.Minute)
	case "hour":
		return base.Add(time.Duration(amount) * time.Hour)
	case "day":
		return base.AddDate(0, 0, amount)
	case "week":
		return base.AddDate(0, 0, 7*amount)
	default:
		return base
	}
}

func formatDate(value time.Time, format string) string {
	var output strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			output.WriteByte(format[i])
			continue
		}
		i++
		directive := format[i]
		if directive >= '1' && directive <= '9' && i+1 < len(format) && format[i+1] == 'N' {
			width := int(directive - '0')
			i++
			nanos := fmt.Sprintf("%09d", value.Nanosecond())
			output.WriteString(nanos[:width])
			continue
		}
		if directive == ':' && i+1 < len(format) && format[i+1] == 'z' {
			i++
			_, offset := value.Zone()
			sign := "+"
			if offset < 0 {
				sign, offset = "-", -offset
			}
			fmt.Fprintf(&output, "%s%02d:%02d", sign, offset/3600, offset%3600/60)
			continue
		}
		layouts := map[byte]string{
			'%': "%", 'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'e': "_2",
			'H': "15", 'M': "04", 'S': "05", 'T': "15:04:05", 'R': "15:04",
			'F': "2006-01-02", 'a': "Mon", 'A': "Monday", 'b': "Jan", 'B': "January",
			'z': "-0700", 'Z': "MST",
		}
		if directive == 's' {
			output.WriteString(strconv.FormatInt(value.Unix(), 10))
		} else if directive == 'N' {
			fmt.Fprintf(&output, "%09d", value.Nanosecond())
		} else if layout, ok := layouts[directive]; ok {
			output.WriteString(value.Format(layout))
		} else {
			output.WriteByte('%')
			output.WriteByte(directive)
		}
	}
	return output.String()
}
