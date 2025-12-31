package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Database  DatabaseConfig  `mapstructure:"database"`
	API       APIConfig       `mapstructure:"api"`
	Crawler   CrawlerConfig   `mapstructure:"crawler"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	LogLevel  string          `mapstructure:"log_level"`
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// APIConfig holds API server settings
type APIConfig struct {
	Port       int             `mapstructure:"port"`
	Debug      bool            `mapstructure:"debug"`
	CORS       bool            `mapstructure:"cors"`
	CORSOrigin string          `mapstructure:"cors_origin"`
	Limits     APILimitsConfig `mapstructure:"limits"`
}

// APILimitsConfig holds query limits for different API endpoints
type APILimitsConfig struct {
	CategoryMaxLimit int `mapstructure:"category_max_limit"`
	SearchMaxLimit   int `mapstructure:"search_max_limit"`
	ListMaxLimit     int `mapstructure:"list_max_limit"`
	UploaderMaxLimit int `mapstructure:"uploader_max_limit"`
	TagMaxLimit      int `mapstructure:"tag_max_limit"`
}

// CrawlerConfig holds crawler settings
type CrawlerConfig struct {
	Host             string `mapstructure:"host"`
	Cookies          string `mapstructure:"cookies"`
	Proxy            string `mapstructure:"proxy"`
	RetryTimes       int    `mapstructure:"retry_times"`
	WaitForIPUnban   bool   `mapstructure:"wait_for_ip_unban"`
	PageDelaySeconds int    `mapstructure:"page_delay_seconds"` // Delay between page fetches
	APIDelaySeconds  int    `mapstructure:"api_delay_seconds"`  // Delay between API calls
	Offset           int    // Temporary parameter, not from config file
}

// SchedulerConfig holds scheduler settings
type SchedulerConfig struct {
	GallerySyncCron    string `mapstructure:"gallery_sync_cron"`
	GallerySyncEnabled bool   `mapstructure:"gallery_sync_enabled"`
	GallerySyncOffset  int    `mapstructure:"gallery_sync_offset"`
	TorrentSyncCron    string `mapstructure:"torrent_sync_cron"`
	TorrentSyncEnabled bool   `mapstructure:"torrent_sync_enabled"`
	ResyncCron         string `mapstructure:"resync_cron"`
	ResyncEnabled      bool   `mapstructure:"resync_enabled"`
	ResyncHours        int    `mapstructure:"resync_hours"`
}

var globalConfig *Config

// Load loads configuration from file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("api.port", 8880)
	v.SetDefault("api.debug", false)
	v.SetDefault("api.cors", true)
	v.SetDefault("api.cors_origin", "*")
	v.SetDefault("api.limits.category_max_limit", 25)
	v.SetDefault("api.limits.search_max_limit", 25)
	v.SetDefault("api.limits.list_max_limit", 25)
	v.SetDefault("api.limits.uploader_max_limit", 25)
	v.SetDefault("api.limits.tag_max_limit", 25)
	v.SetDefault("crawler.host", "e-hentai.org")
	v.SetDefault("crawler.retry_times", 3)
	v.SetDefault("crawler.wait_for_ip_unban", false)
	v.SetDefault("crawler.page_delay_seconds", 1)
	v.SetDefault("crawler.api_delay_seconds", 1)
	v.SetDefault("scheduler.gallery_sync_cron", "0 * * * *")
	v.SetDefault("scheduler.gallery_sync_enabled", true)
	v.SetDefault("scheduler.gallery_sync_offset", 0)
	v.SetDefault("scheduler.torrent_sync_cron", "5 * * * *")
	v.SetDefault("scheduler.torrent_sync_enabled", true)
	v.SetDefault("scheduler.resync_cron", "0 0 * * *")
	v.SetDefault("scheduler.resync_enabled", false)
	v.SetDefault("scheduler.resync_hours", 24)
	v.SetDefault("log_level", "info")

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}

	// Read environment variables
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

// Get returns the global configuration
func Get() *Config {
	return globalConfig
}
