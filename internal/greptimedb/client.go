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

func OpenFromEnv(log logr.Logger) (*gorm.DB, error) {
	retryCount := 0

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
		return nil, errors.New("GREPTIME_HOST and GREPTIME_AUTH environment variables must be set to enable GreptimeDB client")
	}
	greptimedbUserPassword := strings.Split(greptimeAuth, "=")
	greptimedbUser := greptimedbUserPassword[0]
	greptimedbPassword := greptimedbUserPassword[1]
	if len(greptimedbUser) == 0 || len(greptimedbPassword) == 0 {
		return nil, errors.New("GreptimeDB user and password must me set")
	}

	if len(greptimedbUserPassword) != 2 {
		return nil, errors.New("GREPTIME_AUTH must be in the format 'username:password'")
	}

	log.Info("Initializing GreptimeDB client", "host", greptimeHost, "port", greptimePort)
	operation := func() (*gorm.DB, error) {
		dsn := fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s sslmode=disable", greptimedbUser, greptimedbPassword, greptimeHost, greptimePort, greptimeDatabase)
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
