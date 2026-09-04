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

var relativeDatePattern = regexp.MustCompile(`^([+-]?\d+)\s*(second|minute|hour|day|week)s?(?:\s+(ago))?$`)

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
		case arg == "--help":
			_, _ = fmt.Fprintln(e.Stdout, "usage: date [-u] [-d date] [+format]")
			return 0
		default:
			e.Errorf("unsupported operand %q", arg)
			return 2
		}
	}
	if dateInput != "" {
		parsed, err := parseDateInput(now, dateInput)
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
	if seconds, ok := strings.CutPrefix(normalized, "@"); ok {
		value, err := strconv.ParseInt(seconds, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(value, 0), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, input, base.Location()); err == nil {
			return parsed, nil
		}
	}
	match := relativeDatePattern.FindStringSubmatch(normalized)
	if match == nil {
		return time.Time{}, fmt.Errorf("unsupported format")
	}
	amount, _ := strconv.Atoi(match[1])
	if match[3] == "ago" {
		amount = -amount
	}
	switch match[2] {
	case "second":
		return base.Add(time.Duration(amount) * time.Second), nil
	case "minute":
		return base.Add(time.Duration(amount) * time.Minute), nil
	case "hour":
		return base.Add(time.Duration(amount) * time.Hour), nil
	case "day":
		return base.AddDate(0, 0, amount), nil
	case "week":
		return base.AddDate(0, 0, 7*amount), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported unit")
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
		} else if layout, ok := layouts[directive]; ok {
			output.WriteString(value.Format(layout))
		} else {
			output.WriteByte('%')
			output.WriteByte(directive)
		}
	}
	return output.String()
}
