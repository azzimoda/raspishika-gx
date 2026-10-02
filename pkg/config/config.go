package config

import (
	"os"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/database"

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
	KeyDBDriver       = "db_driver"
	KeyDBHost         = "db_host"
	KeyDBPort         = "db_port"
	KeyDBUser         = "db_user"
	KeyDBPassword     = "db_password"
	KeyDBName         = "db_name"
	KeyDBSSLMode      = "db_sslmode"
	KeyDBAutoMigrate  = "db_auto_migrate"

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

	KeyVKToken      = "vk_group_token"
	KeyVKGroupID    = "vk_group_id"
	KeyVKAPIVersion = "vk_api_version"

	KeyProxySourceURL   = "proxy_source_url"
	KeyJustrayProxyAddr = "justray_proxy_addr"
	KeyProxyBanCooldown = "proxy_ban_cooldown"

	KeyJustrayBin              = "justray_bin"
	KeyJustrayProbeProxy       = "justray_probe_proxy"
	KeyJustrayProbeURL         = "justray_probe_url"
	KeyJustrayCheckInterval    = "justray_check_interval"
	KeyJustrayFailureThreshold = "justray_failure_threshold"
	KeyJustrayCooldown         = "justray_cooldown"
	KeyJustrayMaxRotations     = "justray_max_rotations"
	KeyJustrayLongBackoff      = "justray_long_backoff"
	KeyJustrayExclude          = "justray_exclude"

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

// dockerEnvPath is where Docker leaves a marker file in the container. It is a
// variable so tests can point it at a temporary file.
var dockerEnvPath = "/.dockerenv"

// runningInContainer reports whether the process looks containerized, where the
// configuration is injected by the environment. Podman sets $container instead
// of creating the marker file.
func runningInContainer() bool {
	if _, err := os.Stat(dockerEnvPath); err == nil {
		return true
	}
	return os.Getenv("container") != ""
}

func Init() {
	// Defaults
	viper.SetDefault(KeyLogLevel, "trace")
	viper.SetDefault(KeyLogDir, "storage/logs")

	viper.SetDefault(KeyCacheDir, "storage/cache")

	viper.SetDefault(KeyDBFile, "storage/database/data.db")
	viper.SetDefault(KeyDBMigrationDir, "migrations")
	viper.SetDefault(KeyDBDriver, "sqlite3")
	viper.SetDefault(KeyDBHost, "db")
	viper.SetDefault(KeyDBPort, "5432")
	viper.SetDefault(KeyDBUser, "postgres")
	viper.SetDefault(KeyDBPassword, "raspishika")
	viper.SetDefault(KeyDBName, "raspishika")
	viper.SetDefault(KeyDBSSLMode, "disable")
	// Single-process defaults migrate on open. Compose runs the migrations once
	// in the migrate service and sets this to false everywhere else.
	viper.SetDefault(KeyDBAutoMigrate, true)

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

	viper.SetDefault(KeyScheduleTemplateFile, "templates/schedule.html")
	viper.SetDefault(KeyScheduleTemplateDarkFile, "templates/schedule.html")

	viper.SetDefault(KeyProxySourceURL, "https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json")
	viper.SetDefault(KeyJustrayProxyAddr, "127.0.0.1:10808")
	viper.SetDefault(KeyProxyBanCooldown, 5*time.Minute)

	viper.SetDefault(KeyJustrayBin, "justray")
	// The rotator runs on the host, so it probes justray's loopback in-bound.
	// It must not reuse KeyJustrayProxyAddr: in Docker that key is
	// host.docker.internal, which only resolves inside the containers, and a
	// probe that cannot resolve its proxy would rotate a perfectly healthy
	// node every cooldown.
	viper.SetDefault(KeyJustrayProbeProxy, "127.0.0.1:10808")
	// Keep in sync with justrayrotate.DefaultProbeURL. A Bot API method, not the
	// bare host: the root answers 302 to core.telegram.org.
	viper.SetDefault(KeyJustrayProbeURL, "https://api.telegram.org/bot0/getMe")
	viper.SetDefault(KeyJustrayCheckInterval, 20*time.Second)
	viper.SetDefault(KeyJustrayFailureThreshold, 3)
	viper.SetDefault(KeyJustrayCooldown, 60*time.Second)
	viper.SetDefault(KeyJustrayMaxRotations, 3)
	viper.SetDefault(KeyJustrayLongBackoff, 10*time.Minute)
	viper.SetDefault(KeyJustrayExclude, "")

	viper.SetDefault(KeyVKAPIVersion, "5.199")

	// Environment variables
	//
	// The load itself stays unconditional: the justray rotator runs on the host
	// out of the repo directory and needs .env to find its settings. Only the
	// warning is conditional. In a container the configuration comes from
	// compose's `environment:`, so a missing .env is expected rather than
	// misconfiguration, and warning on every start was noise that also broke
	// `docker compose logs | grep -i error` as a smoke check.
	if err := godotenv.Load(); err != nil && !runningInContainer() {
		log.Warn().Err(err).Msg(".env file not found")
	}
	viper.AutomaticEnv()
	// An explicitly empty variable must be readable as empty rather than falling
	// back to the default. JUSTRAY_PROXY_ADDR= is the documented way to run with
	// the free-proxy source only, and without this the default 127.0.0.1:10808
	// came back and justray was used anyway.
	viper.AllowEmptyEnv(true)
}

// DBConfig builds the database configuration from the environment.
//
// The commands repeat this mapping, and cmd/migrate must see exactly the same
// database as the services it prepares, so it lives in one place.
func DBConfig() database.Config {
	return database.Config{
		Driver:        viper.GetString(KeyDBDriver),
		File:          viper.GetString(KeyDBFile),
		MigrationsDir: viper.GetString(KeyDBMigrationDir),
		Host:          viper.GetString(KeyDBHost),
		Port:          viper.GetString(KeyDBPort),
		User:          viper.GetString(KeyDBUser),
		Password:      viper.GetString(KeyDBPassword),
		Name:          viper.GetString(KeyDBName),
		SSLMode:       viper.GetString(KeyDBSSLMode),
		AutoMigrate:   viper.GetBool(KeyDBAutoMigrate),
	}
}
