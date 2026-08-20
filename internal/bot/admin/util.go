package adminbot

import (
	"regexp"
	"strconv"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func parsePeriod(str string) (time.Duration, bool) {
	if str == "" {
		return 0, false
	}

	re := regexp.MustCompile(`^(\d+)\s*(h|d|w|m|y)?$`)
	matches := re.FindStringSubmatch(str)
	multiplier := time.Hour
	switch matches[2] {
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
