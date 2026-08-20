package service

import (
	"context"
	"fmt"

	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/repository"
)

func NewServices(ctx context.Context, container *repository.Container, scraperAPI APIClient) (*Services, error) {

	browser, err := browser.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}

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
	s.Proxy.Stop()
	return s.Browser.Close()
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
	if err := s.Stats.HealthCheck(); err != nil {
		return fmt.Errorf("stats: %w", err)
	}
	return nil
}
