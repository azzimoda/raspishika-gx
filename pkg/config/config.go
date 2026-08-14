package config

import (
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	KeyLogLevel = "log_level"
	KeyLogDir   = "log_dir"

	KeyCacheDir = "cache_dir"

	KeyDBFile         = "db_file"
	KeyDBMigrationDir = "db_migration_dir"

	KeyRedisHost     = "redis_host"
	KeyRedisPort     = "redis_port"
	KeyRedisDB       = "redis_db"
	KeyRedisPassword = "redis_password"

	KeyScraperHost = "scraper_host"
	KeyScraperPort = "scraper_port"

	KeyBrowserHeadless        = "browser_headless"
	KeyBrowserTimeout         = "browser_timeout"
	KeyBrowserWidth           = "browser_width"
	KeyBrowserHeight          = "browser_height"
	KeyBrowserScale           = "browser_scale"
	KeyBrowserRestartInterval = "browser_restart_interval"

	KeyBotToken      = "bot_token"
	KeyAdminBotToken = "admin_bot_token"
	KeyAdminID       = "admin_id"

	KeyBotCommands      = "bot_commands"
	KeyAdminBotCommands = "admin_bot_commands"

	KeyScreenshotDir    = "screenshot_dir"
	KeyChatStateTTL     = "chat_state_ttl"
	KeyCacheScheduleTTL = "cache_schedule_ttl"

	KeyDailyBroadcast   = "daily_broadcast"
	KeyPairNotification = "pair_notification"
	KeyChangeAlert      = "change_alert"

	KeyHandleVacation = "handle_vacation"

	KeyPairNotificationTTL   = "pair_notification_ttl"
	KeyUpdateMonitorInterval = "update_monitor_interval"

	KeyScheduleTemplate         = "schedule_template"
	KeyScheduleTemplateDark     = "schedule_template_dark"
	KeyScheduleTemplateFile     = "schedule_template_file"
	KeyScheduleTemplateDarkFile = "schedule_template_dark_file"
)

func Init() error {
	// Defaults
	viper.SetDefault(KeyLogLevel, "trace")
	viper.SetDefault(KeyLogDir, "storage/logs")

	viper.SetDefault(KeyCacheDir, "storage/cache")

	viper.SetDefault(KeyDBFile, "storage/database/data.db")
	viper.SetDefault(KeyDBMigrationDir, "migrations")

	viper.SetDefault(KeyRedisHost, "redis")
	viper.SetDefault(KeyRedisPort, "6379")
	viper.SetDefault(KeyRedisDB, 1)
	viper.SetDefault(KeyRedisPassword, "")

	viper.SetDefault(KeyScraperHost, "localhost")
	viper.SetDefault(KeyScraperPort, "8080")

	viper.SetDefault(KeyBrowserHeadless, true)
	viper.SetDefault(KeyBrowserTimeout, 30*time.Second)
	viper.SetDefault(KeyBrowserWidth, 1920)
	viper.SetDefault(KeyBrowserHeight, 1080)
	viper.SetDefault(KeyBrowserScale, 1.0)
	viper.SetDefault(KeyBrowserRestartInterval, 24*time.Hour)

	viper.SetDefault(KeyScreenshotDir, "storage/screenshots")
	viper.SetDefault(KeyChatStateTTL, 10*time.Minute)
	viper.SetDefault(KeyCacheScheduleTTL, 30*time.Minute)

	viper.SetDefault(KeyDailyBroadcast, true)
	viper.SetDefault(KeyPairNotification, true)
	viper.SetDefault(KeyChangeAlert, true)

	viper.SetDefault(KeyHandleVacation, true)

	viper.SetDefault(KeyPairNotificationTTL, 90*time.Minute)
	viper.SetDefault(KeyUpdateMonitorInterval, 25*time.Minute)

	viper.SetDefault(KeyScheduleTemplateFile, "templates/light.html")
	viper.SetDefault(KeyScheduleTemplateDarkFile, "templates/dark.html")

	// Environment variables
	if err := godotenv.Load(); err != nil {
		log.Warn().Err(err).Msg(".env file not found")
	}
	viper.AutomaticEnv()

	return nil
}
