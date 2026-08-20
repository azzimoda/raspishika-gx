package service

import (
	"context"
	"strings"
	"sync"
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
	stopOnce     sync.Once
	stopMu       sync.Mutex
	stopped      bool
}

func (s *BotService) Log() *zerolog.Logger {
	return new(log.Logger.With().Str("bot", s.username).Logger())
}

func (s *BotService) Username() string { return s.username }

func (s *BotService) HealthCheck() error {

	s.stopMu.Lock()
	bot := s.Bot
	botCtx := s.botCtx
	s.stopMu.Unlock()

	if bot == nil {
		return nil
	}
	if _, err := bot.GetMe(botCtx); err != nil {
		return err
	}
	return nil
}

func (s *BotService) Start(ctx context.Context) {

	s.superCtx = ctx

	s.stopMu.Lock()
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)
	botCtx := s.botCtx
	s.stopMu.Unlock()

	go s.startBot(ctx, botCtx)

	for {
		select {
		case <-ctx.Done():
			s.Log().Debug().Msg("Context cancelled, stopping bot")
			return
		case _, ok := <-s.restartChan:
			if !ok {
				s.Log().Error().Msg("Restart channel closed, stopping bot...")
				return
			}
			s.Log().Trace().Msg("Restarting bot...")
			s.stopMu.Lock()
			cancel := s.botCtxCancel
			s.botCtx, s.botCtxCancel = context.WithCancel(ctx)
			botCtx = s.botCtx
			s.Bot = nil
			s.stopMu.Unlock()

			cancel()
			s.startBot(ctx, botCtx)
		}
	}
}

var restartSF = singleflight.Group{}

func (s *BotService) Restart() {
	restartSF.Do("restart", func() (any, error) {
		s.Log().Trace().Msg("Sending restart signal...")
		s.stopMu.Lock()
		if s.stopped {
			s.stopMu.Unlock()
			s.Log().Debug().Msg("Bot is stopped, skipping restart")
			return nil, nil
		}
		select {
		case s.restartChan <- struct{}{}:
			s.stopMu.Unlock()
			if s.onRestart != nil {
				s.onRestart(s.superCtx)
			}
		default:
			s.stopMu.Unlock()
			s.Log().Debug().Msg("Bot is not ready for restart, skipping")
		}
		return nil, nil
	})
}

func (s *BotService) Stop() {

	s.stopOnce.Do(func() {
		s.Log().Trace().Msg("Stopping bot...")
		s.stopMu.Lock()
		cancel := s.botCtxCancel
		s.stopped = true
		close(s.restartChan)
		s.stopMu.Unlock()

		if cancel != nil {
			cancel()
		}
	})
}

func (s *BotService) OnRestart(f func(context.Context)) { s.onRestart = f }

func (s *BotService) startBot(ctx context.Context, botCtx context.Context) {

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

		s.stopMu.Lock()
		s.Bot = b
		bot := s.Bot
		s.stopMu.Unlock()

		bot.Start(botCtx)

		return
	}
}

func (s *BotService) handleAPIError(err error) {
	if strings.Contains(err.Error(), "socks connect") || strings.Contains(err.Error(), "connection refused") {
		s.Log().Error().Err(err).Msg("Proxy connection refused, restarting...")
		s.Restart()
	} else {
		s.Log().Error().Err(err).Msg("Telegram API error")
	}
}
