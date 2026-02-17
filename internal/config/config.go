package config

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultEndpoint = "api.rootly.com"
	configDir       = ".rootly-tui"
	configFile      = "config.yaml"
)

type Config struct {
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
	Timezone string `yaml:"timezone"`
	Language string `yaml:"language"`
	Layout   string `yaml:"layout"`
}

const DefaultTimezone = "UTC"
const DefaultLanguage = "en_US"
const DefaultLayout = "horizontal"

// Layout constants
const (
	LayoutHorizontal = "horizontal"
	LayoutVertical   = "vertical"
)

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir)
}

func Path() string {
	return filepath.Join(Dir(), configFile)
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("config file not found")
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}

	if cfg.Timezone == "" {
		cfg.Timezone = DefaultTimezone
	}

	if cfg.Language == "" {
		cfg.Language = DefaultLanguage
	}

	if cfg.Layout == "" {
		cfg.Layout = DefaultLayout
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}

	if cfg.Timezone == "" {
		cfg.Timezone = DefaultTimezone
	}

	if cfg.Language == "" {
		cfg.Language = DefaultLanguage
	}

	if cfg.Layout == "" {
		cfg.Layout = DefaultLayout
	}

	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(Path(), data, 0600)
}

func (c *Config) IsValid() bool {
	return c.APIKey != "" && c.Endpoint != ""
}

// DetectTimezone returns the local system timezone name.
// Falls back to UTC if detection fails.
func DetectTimezone() string {
	// First check TZ environment variable (works on all platforms)
	if tz := os.Getenv("TZ"); tz != "" {
		// Validate it's a real timezone
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}

	// On macOS/Linux, /etc/localtime is a symlink to the timezone file
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		// Link looks like: /var/db/timezone/zoneinfo/America/Los_Angeles
		// or: /usr/share/zoneinfo/America/Los_Angeles
		// Extract the timezone part after "zoneinfo/"
		const marker = "zoneinfo/"
		if idx := strings.Index(link, marker); idx != -1 {
			tz := link[idx+len(marker):]
			// Validate it's a real timezone
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}

	// Try to infer from local time offset (works on Windows and as fallback)
	// This maps common offsets to IANA timezone names
	_, offset := time.Now().Zone()
	offsetHours := offset / 3600
	tzByOffset := map[int]string{
		-12: "Pacific/Kwajalein",
		-11: "Pacific/Pago_Pago",
		-10: "Pacific/Honolulu",
		-9:  "America/Anchorage",
		-8:  "America/Los_Angeles",
		-7:  "America/Denver",
		-6:  "America/Chicago",
		-5:  "America/New_York",
		-4:  "America/Halifax",
		-3:  "America/Sao_Paulo",
		0:   "Europe/London",
		1:   "Europe/Paris",
		2:   "Europe/Berlin",
		3:   "Europe/Moscow",
		4:   "Asia/Dubai",
		5:   "Asia/Karachi",
		6:   "Asia/Dhaka",
		7:   "Asia/Bangkok",
		8:   "Asia/Shanghai",
		9:   "Asia/Tokyo",
		10:  "Australia/Sydney",
		11:  "Pacific/Noumea",
		12:  "Pacific/Auckland",
	}
	if tz, ok := tzByOffset[offsetHours]; ok {
		return tz
	}

	return DefaultTimezone
}

// ListTimezones returns a sorted list of available IANA timezone names.
// It reads from the system's zoneinfo directory.
func ListTimezones() []string {
	var timezones []string

	// Common zoneinfo paths
	zonePaths := []string{
		"/usr/share/zoneinfo",
		"/var/db/timezone/zoneinfo",
		"/usr/lib/zoneinfo",
	}

	var zoneDir string
	for _, p := range zonePaths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			zoneDir = p
			break
		}
	}

	if zoneDir == "" {
		// Fallback to common timezones
		return []string{
			"UTC",
			"America/New_York",
			"America/Chicago",
			"America/Denver",
			"America/Los_Angeles",
			"Europe/London",
			"Europe/Paris",
			"Asia/Tokyo",
			"Asia/Shanghai",
			"Australia/Sydney",
		}
	}

	// Walk the zoneinfo directory
	err := filepath.Walk(zoneDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}

		// Get relative path from zoneDir
		rel, err := filepath.Rel(zoneDir, path)
		if err != nil {
			return nil
		}

		// Skip files that don't look like timezones
		if strings.HasPrefix(rel, "+") || strings.HasPrefix(rel, "posix/") ||
			strings.HasPrefix(rel, "right/") || strings.Contains(rel, ".") ||
			rel == "leap-seconds.list" || rel == "leapseconds" ||
			rel == "posixrules" || rel == "localtime" || rel == "Factory" {
			return nil
		}

		// Try to load it as a timezone to validate
		if _, err := time.LoadLocation(rel); err == nil {
			timezones = append(timezones, rel)
		}

		return nil
	})

	if err != nil || len(timezones) == 0 {
		// Fallback
		return []string{"UTC", "America/New_York", "America/Los_Angeles", "Europe/London", "Asia/Tokyo"}
	}

	// Sort alphabetically
	sort.Strings(timezones)

	return timezones
}

// GetLocation returns the time.Location for the configured timezone.
// Falls back to UTC if the timezone is invalid.
func (c *Config) GetLocation() *time.Location {
	if c.Timezone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}
