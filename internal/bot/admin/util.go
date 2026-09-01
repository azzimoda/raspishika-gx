package adminbot

import (
	"fmt"
	"regexp"
	"strconv"
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

func parsePeriod(str string) (time.Duration, bool) {
	if str == "" {
		return 0, false
	}

	re := regexp.MustCompile(`^(\d+)\s*(h|d|w|m|y)?$`)
	matches := re.FindStringSubmatch(str)
	// matches is nil when the string does not match the pattern (no leading number).
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

func (h *handler) reportChat(chat *model.Chat) reporter.ReportBuilder {

	r := h.Report()
	if chat == nil {
		return r
	}
	return r.Debug("chatID", chat.TgChatID).Debug("username", refutil.DerefOrTypeDefault(chat.UserName)).
		Debug("group", refutil.DerefOrTypeDefault(chat.GroupName))
}
