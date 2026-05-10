package reporter

import (
	"github.com/go-telegram/bot"
)

type Reporter interface{ Report() ReportBuilder }

func NewReporter(b *bot.Bot, recipientChatID int64) Reporter {
	return &reporter{bot: b, recipient: recipientChatID}
}

type reporter struct {
	bot       *bot.Bot
	recipient int64 // Admin chat ID
}

func (r *reporter) Report() ReportBuilder { return NewReportBuilder(r.bot, r.recipient) }
