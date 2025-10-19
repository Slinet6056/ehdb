package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slinet/ehdb/internal/config"
	"go.uber.org/zap"
)

var pool *pgxpool.Pool

// Init initializes the database connection pool
func Init(cfg *config.DatabaseConfig, logger *zap.Logger) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("unable to parse database config: %w", err)
	}

	// Configure pool settings
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	logger.Info("database connection pool initialized",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.DBName),
	)

	return nil
}

// GetPool returns the database connection pool
func GetPool() *pgxpool.Pool {
	return pool
}

// Close closes the database connection pool
func Close() {
	if pool != nil {
		pool.Close()
	}
}
