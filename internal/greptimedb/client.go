package greptimedb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const defaultDatabaseName = "public"

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

func OpenFromEnv(log logr.Logger) (*gorm.DB, error) {
	cfg, err := loadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return openEnsuredDatabase(cfg, log)
}

func DatabaseNameFromEnv() string {
	database := os.Getenv("GREPTIME_DATABASE")
	if database == "" {
		return defaultDatabaseName
	}
	return database
}

func loadConfigFromEnv() (Config, error) {
	greptimeHost := os.Getenv("GREPTIME_HOST")
	greptimePort := os.Getenv("GREPTIME_PORT")
	if greptimePort == "" {
		greptimePort = "4003"
	}
	greptimeDatabase := os.Getenv("GREPTIME_DATABASE")
	if greptimeDatabase == "" {
		greptimeDatabase = defaultDatabaseName
	}
	greptimeAuth := os.Getenv("GREPTIME_AUTH")
	if greptimeHost == "" || greptimeAuth == "" {
		return Config{}, errors.New("GREPTIME_HOST and GREPTIME_AUTH environment variables must be set to enable GreptimeDB client")
	}
	greptimedbUserPassword := strings.SplitN(greptimeAuth, "=", 2)
	if len(greptimedbUserPassword) != 2 {
		return Config{}, errors.New("GREPTIME_AUTH must be in the format 'username=password'")
	}
	greptimedbUser := greptimedbUserPassword[0]
	greptimedbPassword := greptimedbUserPassword[1]
	if len(greptimedbUser) == 0 || len(greptimedbPassword) == 0 {
		return Config{}, errors.New("GreptimeDB user and password must me set")
	}

	return Config{
		Host:     greptimeHost,
		Port:     greptimePort,
		Database: greptimeDatabase,
		User:     greptimedbUser,
		Password: greptimedbPassword,
	}, nil
}

func openEnsuredDatabase(cfg Config, log logr.Logger) (*gorm.DB, error) {
	return openEnsuredDatabaseWith(cfg, log, func(database string) (*gorm.DB, error) {
		return openDatabase(cfg, database, log)
	}, ensureDatabase)
}

func openEnsuredDatabaseWith(
	cfg Config,
	log logr.Logger,
	openFn func(database string) (*gorm.DB, error),
	ensureFn func(db *gorm.DB, database string) error,
) (*gorm.DB, error) {
	bootstrapDB, err := openFn(defaultDatabaseName)
	if err != nil {
		return nil, err
	}
	if cfg.Database == defaultDatabaseName {
		return bootstrapDB, nil
	}

	if err := ensureFn(bootstrapDB, cfg.Database); err != nil {
		return nil, fmt.Errorf("ensure GreptimeDB database %s: %w", cfg.Database, err)
	}

	targetDB, err := openFn(cfg.Database)
	if err != nil {
		return nil, err
	}
	return targetDB, nil
}

func openDatabase(cfg Config, database string, log logr.Logger) (*gorm.DB, error) {
	retryCount := 0

	log.Info("Initializing GreptimeDB client", "host", cfg.Host, "port", cfg.Port, "database", database)
	operation := func() (*gorm.DB, error) {
		dsn := fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s sslmode=disable", cfg.User, cfg.Password, cfg.Host, cfg.Port, database)
		greptimeDb, err := gorm.Open(postgres.New(postgres.Config{
			DSN:              dsn,
			WithoutReturning: true,
		}), &gorm.Config{
			DisableAutomaticPing: true,
		})
		if err != nil {
			retryCount++
			log.Error(err, "Failed to initialize Greptime client. Retrying...")
			return nil, err
		}
		return greptimeDb, nil
	}

	exponentialBackoff := backoff.NewExponentialBackOff()
	exponentialBackoff.InitialInterval = 5 * time.Second //nolint:mnd

	notifyFunc := func(err error, duration time.Duration) {
		log.Error(err, "Failed to initialize Greptime client. Retrying...", "retry_count", retryCount, "duration", duration.String())
	}

	db, err := backoff.Retry(context.TODO(), operation,
		backoff.WithBackOff(exponentialBackoff),
		backoff.WithMaxElapsedTime(3*time.Minute), //nolint:mnd
		backoff.WithNotify(notifyFunc),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Greptime client after retries: %w", err)
	}

	return db, nil
}

func ensureDatabase(db *gorm.DB, database string) error {
	return db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteIdentifier(database))).Error
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
