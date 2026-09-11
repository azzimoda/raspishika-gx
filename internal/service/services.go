package service

import (
	"context"
	"fmt"

	"github.com/spf13/viper"

	"github.com/azzimoda/go-tg-proxy/proxy"
	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/proxyfail"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/config"
)

func newProxyService() (*proxy.Service, *proxyfail.FailTracker) {
	tracker := proxyfail.NewFailTracker(viper.GetDuration(config.KeyProxyBanCooldown))
	service := proxy.NewService(
		proxy.NewProxiflySource(viper.GetString(config.KeyProxySourceURL)),
		proxy.WithChecker(proxyfail.BanChecker{
			Checker: proxy.TelegramChecker{},
			Tracker: tracker,
		}),
	)
	return service, tracker
}

func NewServices(ctx context.Context, container *repository.Container, scraperAPI APIClient) (*Services, error) {

	browser, err := browser.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}

	proxyService, proxyTracker := newProxyService()

	return &Services{
		Browser:          browser,
		Proxy:            proxyService,
		ProxyFailTracker: proxyTracker,
		Chat:             NewChatService(container.Chat),
		Schedule:         NewScheduleService(scraperAPI, browser, container.Schedule),
		Stats:            NewStatsService(container.Log, container.Chat),
	}, nil
}

// NewAdminServices builds the services the admin bot needs (proxy, chat,
// schedule lookups, statistics) without the screenshot browser.
func NewAdminServices(ctx context.Context, container *repository.Container, scraperAPI APIClient) (*Services, error) {
	proxyService, proxyTracker := newProxyService()

	return &Services{
		Proxy:            proxyService,
		ProxyFailTracker: proxyTracker,
		Chat:             NewChatService(container.Chat),
		Schedule:         NewScheduleService(scraperAPI, nil, container.Schedule),
		Stats:            NewStatsService(container.Log, container.Chat),
	}, nil
}

type Services struct {
	Browser          *browser.ChromedpBrowser
	Proxy            *proxy.Service
	ProxyFailTracker *proxyfail.FailTracker
	Chat             *ChatService
	Schedule         *ScheduleService
	Stats            *StatsService
}

func (s *Services) Stop() error {
	s.Proxy.Stop()
	if s.Browser != nil {
		return s.Browser.Close()
	}
	return nil
}

func (s *Services) HealthCheck() error {
	if s.Browser != nil {
		if err := s.Browser.HealthCheck(); err != nil {
			return fmt.Errorf("browser: %w", err)
		}
	}
	if err := s.Proxy.HealthCheck(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	if err := s.Chat.HealthCheck(); err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	if err := s.Stats.HealthCheck(); err != nil {
		return fmt.Errorf("stats: %w", err)
	}
	return nil
}
