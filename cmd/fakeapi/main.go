package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

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

	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Server exited with error")
	}
}

func run() error {

	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

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
		return fmt.Errorf("failed to create redis client: %w", err)
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	log.Info().Msg("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	<-serverErr

	log.Info().Msg("Done")
	return nil
}
