package mainbot

import (
	"time"

	"github.com/go-telegram/bot"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
	"github.com/azzimoda/raspishika-gx/pkg/reporter"
)

func New(s *service.Services, p string, reporter reporter.Reporter) (*bot.Bot, error) {
	h := newHandler(s, reporter)

	httpClient, err := proxyutil.NewHTTPProxyClient(p)
	if err != nil {
		return nil, err
	}

	opts := []bot.Option{
		bot.WithHTTPClient(30*time.Second, httpClient),
		bot.WithCheckInitTimeout(30 * time.Second),
		bot.WithMiddlewares(
			h.ignoreOldMessage,
			h.ignoreInaccessibleMessageCQ,
			h.syncCQSingleFlight,
			h.ensureChat,
			h.logHandler,
		),
		bot.WithDefaultHandler(h.handleDefault),
	}
	if viper.GetString(config.KeyLogLevel) == "trace" {
		opts = append(opts, bot.WithDebug())
	}
	b, err := bot.New(viper.GetString(config.KeyBotToken), opts...)
	if err != nil {
		return nil, err
	}

	h.registerHandlers(b)
	return b, nil
}
