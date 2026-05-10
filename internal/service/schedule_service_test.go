package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/scraper"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/testutil"
)

func TestScheduleService_EnsureGroups(t *testing.T) {
	if err := testutil.MoveToProjectRoot(); err != nil {
		t.Fatal(err)
	}

	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	viper.Set(config.KeyProxyListFile, "./storage/proxies.json")

	artifactDir := t.ArtifactDir()
	viper.Set(config.KeyDBFile, filepath.Join(artifactDir, "database/data.db"))
	viper.Set(config.KeyCacheDir, filepath.Join(artifactDir, "cache"))
	viper.Set(config.KeyScreenshotDir, filepath.Join(artifactDir, "screenshots"))
	viper.Set(config.KeyLogDir, filepath.Join(artifactDir, "logs"))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	container, err := repository.NewContainer(db)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	browser, err := browser.New()
	scraper := scraper.New(browser)
	scheduleService := service.NewScheduleService(browser, scraper, container.Schedule, container.Group)

	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		pre     func() // setup function to run before test case
		want    bool
		wantErr bool
	}{
		{
			name:    "No data, needs update",
			want:    true,
			wantErr: false,
		},
		{
			name:    "Up to date",
			want:    false,
			wantErr: false,
		},
		{
			name:    "No data again",
			pre:     func() { scheduleService.DeleteAllGroups(context.Background()) },
			want:    true,
			wantErr: false,
		},
		{
			name:    "Up to date again",
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pre != nil {
				tt.pre()
			}

			got, gotErr := scheduleService.EnsureGroups(context.Background())

			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("EnsureGroups() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("EnsureGroups() succeeded unexpectedly")
			}

			if got != tt.want {
				t.Errorf("EnsureGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}
