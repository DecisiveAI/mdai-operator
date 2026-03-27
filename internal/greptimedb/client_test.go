package greptimedb

import (
	"testing"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
)

func TestDatabaseNameFromEnv(t *testing.T) {
	t.Run("default public", func(t *testing.T) {
		t.Setenv("GREPTIME_DATABASE", "")
		if got := DatabaseNameFromEnv(); got != defaultDatabaseName {
			t.Fatalf("expected default database %q, got %q", defaultDatabaseName, got)
		}
	})

	t.Run("configured database", func(t *testing.T) {
		t.Setenv("GREPTIME_DATABASE", "mdai")
		if got := DatabaseNameFromEnv(); got != "mdai" {
			t.Fatalf("expected configured database %q, got %q", "mdai", got)
		}
	})
}

func TestOpenEnsuredDatabaseWith_PublicDatabase(t *testing.T) {
	t.Parallel()

	cfg := Config{Database: defaultDatabaseName}
	var opened []string
	ensureCalled := false

	db, err := openEnsuredDatabaseWith(cfg, logr.Discard(), func(database string) (*gorm.DB, error) {
		opened = append(opened, database)
		return &gorm.DB{}, nil
	}, func(*gorm.DB, string) error {
		ensureCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("openEnsuredDatabaseWith returned error: %v", err)
	}
	if db == nil {
		t.Fatal("expected database handle")
	}
	if len(opened) != 1 || opened[0] != defaultDatabaseName {
		t.Fatalf("expected only bootstrap database to be opened, got: %v", opened)
	}
	if ensureCalled {
		t.Fatal("did not expect ensureDatabase to be called for public database")
	}
}

func TestOpenEnsuredDatabaseWith_TargetDatabase(t *testing.T) {
	t.Parallel()

	cfg := Config{Database: "mdai"}
	var opened []string
	var ensured string

	db, err := openEnsuredDatabaseWith(cfg, logr.Discard(), func(database string) (*gorm.DB, error) {
		opened = append(opened, database)
		return &gorm.DB{}, nil
	}, func(_ *gorm.DB, database string) error {
		ensured = database
		return nil
	})
	if err != nil {
		t.Fatalf("openEnsuredDatabaseWith returned error: %v", err)
	}
	if db == nil {
		t.Fatal("expected database handle")
	}
	if len(opened) != 2 || opened[0] != defaultDatabaseName || opened[1] != "mdai" {
		t.Fatalf("expected bootstrap and target database to be opened, got: %v", opened)
	}
	if ensured != "mdai" {
		t.Fatalf("expected target database to be ensured, got: %q", ensured)
	}
}
