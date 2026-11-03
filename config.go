package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"coincheur/internal/detect"
	"coincheur/internal/scan"
)

const (
	providerEnv  = "COINCHEUR_PROVIDER"
	unitsEnv     = "COINCHEUR_UNITS"
	reportDirEnv = "COINCHEUR_REPORT_DIR"

	displayScore   = 0.35
	confirmScore   = 0.55
	confirmStreak  = 2
	previewWidth   = 640
	previewEvery   = 50 * time.Millisecond
	defaultUnits   = "ALL"
	windowTitle    = "Coincheur"
	windowWidth    = 1100
	windowHeight   = 645
	windowMinWidth = 760
)

func detectConfig() (detect.Config, error) {
	library, err := detect.LocateLibrary()
	if err != nil {
		return detect.Config{}, err
	}

	model, err := detect.LocateModel()
	if err != nil {
		return detect.Config{}, err
	}

	return detect.Config{
		LibraryPath:  library,
		ModelPath:    model,
		Provider:     envOr(providerEnv, detect.DefaultProvider()),
		ComputeUnits: envOr(unitsEnv, defaultUnits),
		MinScore:     displayScore,
	}, nil
}

func scanConfig() scan.Config {
	return scan.Config{
		ConfirmScore:    confirmScore,
		PreviewWidth:    previewWidth,
		PreviewInterval: previewEvery,
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

// reportDir is where error reports are kept, in the user's application support folder unless overridden.
func reportDir() (string, error) {
	if folder := os.Getenv(reportDirEnv); folder != "" {
		return folder, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating the user config folder: %w", err)
	}

	return filepath.Join(base, "coincheur", "reports"), nil
}

func reportSettings() map[string]string {
	return map[string]string{
		"displayScore":  strconv.FormatFloat(displayScore, 'f', -1, 32),
		"confirmScore":  strconv.FormatFloat(confirmScore, 'f', -1, 32),
		"confirmStreak": strconv.Itoa(confirmStreak),
		"provider":      envOr(providerEnv, detect.DefaultProvider()),
		"units":         envOr(unitsEnv, defaultUnits),
	}
}
