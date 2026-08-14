package service

import (
	"context"
	"fmt"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
)

func NewServices(ctx context.Context, container *repository.Container) (*Services, error) {
	browser, err := browser.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}

	scraperAddr := fmt.Sprintf("%s:%s",
		viper.GetString(config.KeyScraperHost), viper.GetString(config.KeyScraperPort))
	scraperAPI := apiclient.New(scraperAddr)

	return &Services{
		Browser:  browser,
		Proxy:    NewProxyService(container.Proxy),
		Chat:     NewChatService(container.Chat),
		Schedule: NewScheduleService(scraperAPI, browser, container.Schedule),
		Stats:    NewStatsService(container.Log, container.Chat),
	}, nil
}

type Services struct {
	Browser  *browser.ChromedpBrowser
	Proxy    *ProxyService
	Chat     *ChatService
	Schedule *ScheduleService
	Stats    *StatsService
}

func (s *Services) Stop() error {
	s.Browser.Close()
	return nil
}

func (s *Services) HealthCheck() error {
	if err := s.Browser.HealthCheck(); err != nil {
		return fmt.Errorf("browser: %w", err)
	}
	if err := s.Proxy.HealthCheck(); err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	if err := s.Chat.HealthCheck(); err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	if err := s.Schedule.HealthCheck(); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	if err := s.Stats.HealthCheck(); err != nil {
		return fmt.Errorf("stats: %w", err)
	}
	return nil
}
