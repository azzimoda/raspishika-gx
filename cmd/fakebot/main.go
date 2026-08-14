package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	adminbot "github.com/azzimoda/raspishika-gx/internal/bot/admin"
	mainbot "github.com/azzimoda/raspishika-gx/internal/bot/main"
	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/fakescraper"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var fakeScraper = fakescraper.FakeScraper{}

type fakeScraperAPI struct{}

func (fakeScraperAPI) GetDepartments(context.Context) ([]model.Department, error) {
	return fakeScraper.ScrapeDepartments()
}
func (fakeScraperAPI) GetGroup(ctx context.Context, name string) (*model.Group, error) {
	for _, gs := range fakescraper.FakeGroups {
		for _, g := range gs {
			if string(g.GroupName) == name {
				return &g, nil
			}
		}
	}
	return nil, errors.New("not found")
}
func (fakeScraperAPI) GetGroups(ctx context.Context, departmentName string) ([]model.Group, error) {
	return fakeScraper.ScrapeDepartmentGroups(&model.Department{Name: departmentName})
}

func (fakeScraperAPI) GetTeacher(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	log.Debug().Str("target", nameOrID).Msg("Getting teacher by name or ID...")

	for _, t := range fakescraper.FakeTeachers {
		log.Trace().Str("name", t.Name).Str("id", t.TeacherID).Str("target", nameOrID).Send()
		if t.Name == nameOrID || t.TeacherID == nameOrID {
			log.Trace().Msg("Teacher found")
			return &t, nil
		}
	}
	log.Warn().Msg("Teacher not found")
	return nil, errors.New("not found")
}
func (fakeScraperAPI) SearchTeachers(context.Context, string) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}
func (fakeScraperAPI) GetTeachers(context.Context) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}
func (f fakeScraperAPI) GetSchedule(ctx context.Context, params *apiclient.GetScheduleParams) (
	schedule *model.ScheduleData,
	err error,
) {
	log.Debug().Any("params", params).Msg("Getting schedule...")

	var key string
	if params.Group != "" {
		key = params.Group
		log.Trace().Str("name", key).Msg("Group schedule")
	} else if params.Teacher != "" {
		teacher, err := f.GetTeacher(ctx, params.Teacher)
		if err != nil {
			log.Warn().Msg("Teacher not found by ID")
			return nil, err
		}
		key = teacher.Name
		log.Trace().Str("name", key).Msg("Teacher schedule")
	} else {
		panic("invalid schedule config")
	}

	scheduleData, ok := fakescraper.FakeSchedules[key]
	if ok {
		log.Trace().Msg("Schedule found")
		return &scheduleData, nil
	}
	log.Warn().Msg("Schedule not found")
	return nil, fmt.Errorf("no such group/teacher")

}

func main() {
	// INITIALIZE

	if err := config.Init(); err != nil {
		log.Fatal().Err(err).Msg("Config initialization failed")
	}

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open DB")
	}
	defer db.Close()

	container := repository.NewContainer(db)

	appReporter := new(appReporter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	browser, err := browser.New(ctx)
	if err != nil {
		cancel()
		log.Fatal().Err(err).Msg("Failed to initialize browser")
	}
	defer browser.Close()

	scraperAPI := fakeScraperAPI{}
	services := &service.Services{
		Browser:  browser,
		Proxy:    service.NewProxyService(container.Proxy),
		Chat:     service.NewChatService(container.Chat),
		Schedule: service.NewScheduleService(scraperAPI, browser, container.Schedule),
		Stats:    service.NewStatsService(container.Log, container.Chat),
	}

	mainBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return mainbot.New(services, p, appReporter) },
		services.Proxy,
	)
	defer mainBot.Stop()
	adminBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return adminbot.New(services, p, appReporter) },
		services.Proxy,
	)
	defer adminBot.Stop()

	broadcast := service.NewBroadcastService(mainBot, services, appReporter)
	defer broadcast.Stop()

	mainBot.OnRestart(func(ctx context.Context) {
		appReporter.Report().Msg("Main bot is restarting...")

		for mainBot.Bot == nil {
			select {
			case <-ctx.Done():
				log.Warn().Msg("Context cancelled!")
				return
			default:
			}
			log.Debug().Msg("Wating for main bot...")
			time.Sleep(5 * time.Second)
		}

		appReporter.Report().Msg("Main bot has just restarted")
	})
	adminBot.OnRestart(func(ctx context.Context) {
		if adminBot.Bot != nil {
			appReporter.Report().Msg("Admin bot is restarting...")
			time.Sleep(3 * time.Second)
		}

		for adminBot.Bot == nil {
			select {
			case <-ctx.Done():
				log.Warn().Msg("Context cancelled!")
				return
			default:
			}
			log.Debug().Msg("Wating for admin bot...")
			time.Sleep(5 * time.Second)
		}

		appReporter.Report().Msg("Admin bot has just restarted")
	})

	// RUN

	log.Info().Msg("Starting app...")
	ctx, cancel = signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	// Services

	if err := services.HealthCheck(); err != nil {
		log.Fatal().Err(err).Msg("Healthcheck failed")
	}
	broadcast.Run(ctx, service.BroadcastConfig{
		Daily:            viper.GetBool("daily_broadcast"),
		PairNotification: viper.GetBool("pair_notification"),
		ChangeAlert:      viper.GetBool("change_alert"),
	})

	// Bots

	if err := mainBot.HealthCheck(); err != nil {
		log.Fatal().Err(err).Msg("Main bot healthcheck failed")
	}
	go mainBot.Start(ctx)

	if viper.GetInt("admin_id") != 0 {
		if err := adminBot.HealthCheck(); err != nil {
			log.Error().Err(err).Msg("Admin bot health check failed")
		} else {
			shouldReturn := startAdminBot(ctx, mainBot, adminBot, appReporter)
			if shouldReturn {
				return
			}
		}
	} else {
		log.Debug().Msg("Admin bot is disabled")
	}

	<-ctx.Done()
}

func startAdminBot(ctx context.Context, mainBot, adminBot *service.BotService, appReporter *appReporter) bool {
	go adminBot.Start(ctx)
	for adminBot.Bot == nil || mainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return true
		default:
		}
		log.Debug().Msg("Wating for bots...")
		time.Sleep(5 * time.Second)
	}
	time.Sleep(1 * time.Second)
	log.Info().Msg("All bots started")

	appReporter.Reporter = reporter.NewReporter(adminBot.Bot, viper.GetInt64(config.KeyAdminID))
	appReporter.Report().Msg("Started on bot @" + mainbot.GetMe(mainBot.Bot).Username)
	return false
}

type appReporter struct{ reporter.Reporter }

func (r appReporter) Report() reporter.ReportBuilder {
	if r.Reporter == nil {
		return reporter.EmptyReportBuilder()
	}
	return r.Reporter.Report()
}
