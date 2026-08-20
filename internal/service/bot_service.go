package service

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

type BotBuilderFunc = func(proxy string) (*bot.Bot, error)

func NewBotService(builder BotBuilderFunc, proxyService *ProxyService) *BotService {
	return &BotService{builder: builder, proxy: proxyService, restartChan: make(chan struct{})}
}

type BotService struct {
	superCtx context.Context
	username string
	builder  BotBuilderFunc
	*bot.Bot
	proxy        *ProxyService
	botCtx       context.Context
	botCtxCancel context.CancelFunc
	restartChan  chan struct{}
	onRestart    func(context.Context)
}

func (s *BotService) Log() *zerolog.Logger {
	return new(log.Logger.With().Str("bot", s.username).Logger())
}

func (s *BotService) Username() string { return s.username }

func (s *BotService) HealthCheck() error {
	if s.Bot == nil {
		return nil
	}
	if _, err := s.Bot.GetMe(s.botCtx); err != nil {
		return err
	}
	return nil
}

func (s *BotService) Start(ctx context.Context) {
	s.superCtx = ctx
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)

	go s.startBot(ctx)

	for {
		_, ok := <-s.restartChan
		if !ok {
			s.Log().Error().Msg("Restart channel closed, stopping bot...")
			return
		}
		s.Log().Trace().Msg("Restarting bot...")
		s.botCtxCancel()
		s.Bot = nil
		s.startBot(ctx)
	}
}

var restartSF = singleflight.Group{}

func (s *BotService) Restart() {
	restartSF.Do("restart", func() (any, error) {
		s.Log().Trace().Msg("Sending restart signal...")
		s.restartChan <- struct{}{}
		if s.onRestart != nil {
			s.onRestart(s.superCtx)
		}
		return nil, nil
	})
}

func (s *BotService) Stop() {
	s.Log().Trace().Msg("Stopping bot...")
	s.botCtxCancel()
	close(s.restartChan)
}

func (s *BotService) OnRestart(f func(context.Context)) { s.onRestart = f }

func (s *BotService) startBot(ctx context.Context) {
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		proxy, err := s.proxy.FirstAvailable()
		if err != nil {
			s.Log().Error().Err(err).Msg("No available proxy, retrying in 10s...")
			time.Sleep(10 * time.Second)

			select {
			case <-ctx.Done():
				s.Log().Error().Msg("Context cancelled, stopping bot...")
				return
			default:
			}
			continue
		}
		s.Log().Debug().Str("proxy", proxy)

		s.Log().Trace().Msg("Building bot...")
		b, err := s.builder(proxy)
		if err != nil {
			s.Log().Error().Err(err).Msg("Failed to build bot, retrying in 5 second...")
			time.Sleep(5 * time.Second)
			continue
		}

		s.Log().Trace().Msg("Bot built, getting username...")
		if me, err := b.GetMe(ctx); err == nil {
			s.username = me.Username
			s.Log().Info().Msg("Bot started")
		} else {
			s.Log().Error().Err(err).Msg("Failed to get bot username, retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		bot.WithErrorsHandler(func(err error) { go s.handleAPIError(err) })(b)
		s.Log().Trace().Msg("Bot initialized")

		s.Bot = b

		break
	}

	s.Bot.Start(s.botCtx)
}

func (s *BotService) handleAPIError(err error) {
	if strings.Contains(err.Error(), "socks connect") || strings.Contains(err.Error(), "connection refused") {
		s.Log().Error().Err(err).Msg("Proxy connection refused, restarting...")
		s.Restart()
	} else {
		s.Log().Error().Err(err).Msg("Telegram API error")
	}
}
