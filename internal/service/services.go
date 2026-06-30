package service

import (
	"fmt"

	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/scraper"
)

func NewServices(container *repository.Container) (*Services, error) {
	browser, err := browser.New()
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}

	scraper := scraper.New(browser)

	return &Services{
		Browser:  browser,
		Proxy:    NewProxyService(container.Proxy),
		Chat:     NewChatService(container.Chat),
		Schedule: NewScheduleService(browser, scraper, container.Schedule, container.Group),
		Stats:    NewStatsService(container.Log, container.Chat),
		// Log:      NewLogService(container.Log),
	}, nil
}

type Services struct {
	Browser  *browser.BrowserService
	Proxy    *ProxyService
	Chat     *ChatService
	Schedule *ScheduleService
	Stats    *StatsService
	// Log      *LogService
}

func (s *Services) Stop() error {
	// TODO
	s.Browser.Close()
	return nil
}

func (s *Services) HealthCheck() error {
	if err := s.Browser.HealthCheck(); err != nil {
		return err
	}
	if err := s.Proxy.HealthCheck(); err != nil {
		return err
	}
	if err := s.Chat.HealthCheck(); err != nil {
		return err
	}
	if err := s.Schedule.HealthCheck(); err != nil {
		return err
	}
	if err := s.Stats.HealthCheck(); err != nil {
		return err
	}
	return nil
}
