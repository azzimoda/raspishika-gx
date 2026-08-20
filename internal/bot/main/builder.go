package mainbot

import (
	"context"
	_ "embed"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
)

//go:embed commands.yaml
var myCommandsBytes []byte

func New(s *service.Services, proxy string, reporter reporter.Reporter, onActivity func()) (*bot.Bot, error) {
	h := newHandler(s, reporter)

	httpClient, err := proxyutil.NewHTTPProxyClient(proxy)
	if err != nil {
		return nil, err
	}

	touchActivity := func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if onActivity != nil {
				onActivity()
			}
			next(ctx, b, update)
		}
	}

	opts := []bot.Option{
		bot.WithHTTPClient(10*time.Second, httpClient),
		bot.WithCheckInitTimeout(10 * time.Second),
		bot.WithMiddlewares(
			touchActivity,
			h.ignoreOldMessage,
			h.ignoreInaccessibleMessageCQ,
			h.syncCQSingleFlight,
			h.ensureChat,
			h.logUpdate,
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

	myCommands, err := botutil.ParseBotCommands(myCommandsBytes)
	if err != nil {
		panic(err)
	}
	log.Debug().Any("myCommands", myCommands).Msg("Settings my commands...")
	if ok, err := b.SetMyCommands(
		context.Background(),
		&bot.SetMyCommandsParams{Commands: myCommands},
	); err != nil || !ok {
		log.Error().Err(err).Any("myCommands", myCommands).Msg("Failed to bot commands")
	} else {
		log.Debug().Msg("My commands set")
	}

	return b, nil
}
