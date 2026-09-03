package adminbot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

// callbackDataRegexp matches a callback data that starts with command and may
// carry extra "\n"-separated arguments after it.
func callbackDataRegexp(command string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf("^%s(\n.*)*$", command))
}

// statsTZ is the timezone in which the admin provides and interprets calendar
// dates/times. The bot operates in Asia/Yekaterinburg; DB timestamps are stored
// in UTC, so parsed bounds are normalized to UTC before querying.
var statsTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// periodSpec describes the time window for dashboard statistics and how it was
// expressed by the caller (for display).
type periodSpec struct {
	start      time.Time
	end        time.Time
	label      string
	isRelative bool
}

// parsePeriodSpec parses a /dashboard argument describing the statistics window.
// It supports three forms:
//
//	"7d"                 relative window ending now (h/d/w/m/y suffixed number);
//	"2026-08-20..2026-08-27" an inclusive calendar range between two dates;
//	"48h@2026-08-20 12:00"  an offset (h/d/w/m/y) starting at a given moment.
//
// In the range/since forms, bare dates are interpreted as start-of-day in
// statsTZ. For a range, the end is expanded to the last instant of its day so
// the full final day is included. An unparseable input returns ok=false and the
// caller falls back to the default window.
func parsePeriodSpec(str string) (periodSpec, bool) {
	if str == "" {
		return periodSpec{}, false
	}

	// 1) Relative window: "3d", "48h", "2w", "1m", "1y".
	if dur, ok := parseRelativePeriod(str); ok && dur > 0 {
		now := time.Now()
		return periodSpec{
			start:      now.Add(-dur),
			end:        now,
			isRelative: true,
		}, true
	}

	// 2) Range "A..B" (inclusive dates).
	if i := strings.Index(str, ".."); i >= 0 {
		start, ok1 := parseRangeStart(str[:i])
		endRaw := str[i+2:]
		end, ok2 := parseRangeStart(endRaw)
		if !ok1 || !ok2 {
			return periodSpec{}, false
		}
		// Include the whole final day.
		end = end.Add(24*time.Hour - time.Millisecond)
		if !start.Before(end) {
			return periodSpec{}, false
		}
		return periodSpec{
			start: start.UTC(),
			end:   end.UTC(),
			label: fmt.Sprintf("%s → %s", formatDateLocal(start), formatDateLocal(end)),
		}, true
	}

	// 3) Since-offset "48h@2026-08-20 12:00".
	if at := strings.Index(str, "@"); at >= 0 {
		dur, ok := parseRelativePeriod(str[:at])
		if !ok || dur <= 0 {
			return periodSpec{}, false
		}
		moment, ok := parseMoment(strings.TrimSpace(str[at+1:]))
		if !ok {
			return periodSpec{}, false
		}
		return periodSpec{
			start: moment.UTC(),
			end:   moment.Add(dur).UTC(),
			label: fmt.Sprintf("%s from %s", formatDuration(dur), formatMomentLocal(moment)),
		}, true
	}

	return periodSpec{}, false
}

// parseRelativePeriod parses a h/d/w/m/y-suffixed duration such as "48h" or
// "7d" into a time.Duration.
func parseRelativePeriod(str string) (time.Duration, bool) {
	re := regexp.MustCompile(`^(\d+)\s*(h|d|w|m|y)?$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(str))
	if len(matches) < 2 {
		return 0, false
	}
	suffix := ""
	if len(matches) > 2 {
		suffix = matches[2]
	}
	multiplier := time.Hour
	switch suffix {
	case "d":
		multiplier = 24 * time.Hour
	case "w":
		multiplier = 7 * 24 * time.Hour
	case "m":
		multiplier = 30 * 24 * time.Hour
	case "y":
		multiplier = 365 * 24 * time.Hour
	}

	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return time.Duration(multiplier * time.Duration(num)), true
}

// parseRangeStart parses the start bound of a range: a full moment or a bare
// date (which becomes start-of-day in statsTZ).
func parseRangeStart(str string) (time.Time, bool) {
	str = strings.TrimSpace(str)
	if t, ok := parseMoment(str); ok {
		return t, true
	}
	if t, ok := parseDate(str); ok {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, statsTZ), true
	}
	return time.Time{}, false
}

// parseMoment parses a full timestamp in statsTZ, e.g. "2026-08-20 12:00" or
// "2026-08-20T12:00:00".
func parseMoment(str string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, str, statsTZ); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseDate parses a bare calendar date "2006-01-02" (no time component).
func parseDate(str string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", str, statsTZ)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// formatDateLocal renders a time as a local date "2006-01-02".
func formatDateLocal(t time.Time) string {
	return t.In(statsTZ).Format("2006-01-02")
}

// formatMomentLocal renders a time as a local moment "2006-01-02 15:04".
func formatMomentLocal(t time.Time) string {
	return t.In(statsTZ).Format("2006-01-02 15:04")
}

func (h *handler) reportChat(chat *model.Chat) reporter.ReportBuilder {

	r := h.Report()
	if chat == nil {
		return r
	}
	return r.
		Debug("chatID", chat.TgChatID).
		Debug("username", refutil.DerefOrTypeDefault(chat.UserName)).
		Debug("state", chat.State).
		Debug("department", refutil.DerefOrTypeDefault(chat.DepartmentName)).
		Debug("group", refutil.DerefOrTypeDefault(chat.GroupName)).
		Debug("daily_sending", refutil.DerefOrTypeDefault(chat.DailySendingTime)).
		Debug("pair_sending", chat.PairSending).
		Debug("change_alert", chat.ChangeAlert).
		Debug("access", chat.Access).
		Debug("dark_mode", chat.DarkMode).
		Debug("created_at", formatMomentLocal(chat.CreatedAt)).
		Debug("updated_at", formatMomentLocal(chat.UpdatedAt))
}
