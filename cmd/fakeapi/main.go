//go:generate swag init -d ../.. -g cmd/fakeapi/main.go -o ../../docs --parseDependency

// @title           Raspishika API Demo
// @version         1.0
// @description     API для доступа к расписанию МПК ТИУ (демо-версия)
// @host            localhost:8080
// @BasePath        /api/v1

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/api/browser"
	"github.com/azzimoda/raspishika-gx/internal/api/handler"
	"github.com/azzimoda/raspishika-gx/internal/api/router"
	"github.com/azzimoda/raspishika-gx/internal/api/service"
	"github.com/azzimoda/raspishika-gx/internal/fakescraper"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/azzimoda/raspishika-gx/pkg/redisdb"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func main() {
	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	browser, err := browser.New()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create browser")
	}
	defer func() {
		if err := browser.Close(); err != nil {
			log.Error().Err(err).Msg("Browser closed with an error")
		}
	}()

	redisAddr := fmt.Sprintf("%s:%s", viper.GetString(config.KeyRedisHost), viper.GetString(config.KeyRedisPort))
	redisDB := viper.GetInt(config.KeyRedisDB)
	log.Info().Str("addr", redisAddr).Int("db", redisDB).Msg("Connecting to redis...")
	redisClient, err := redisdb.New(&redis.Options{
		Addr:         redisAddr,
		Password:     viper.GetString(config.KeyRedisPassword),
		DB:           redisDB,
		PoolSize:     100,
		MinIdleConns: 10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create redis client")
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Error().Err(err).Msg("Redis client closed with an error")
		}
	}()

	scheduleService := service.NewScheduleService(fakescraper.NewFakeScraper(), redisClient)
	handler := handler.NewHandler(scheduleService)
	engine := router.Init(handler)

	serverAddr := ":" + viper.GetString(config.KeyScraperPort)
	log.Info().Str("addr", serverAddr).Msg("Starting server...")
	srv := &http.Server{Addr: serverAddr, Handler: engine}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Send()
		}
		log.Info().Msg("Server stopped")
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Sutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Done")

}
