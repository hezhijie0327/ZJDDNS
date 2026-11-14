package config

import "encoding/json"

// Example returns the example configuration as indented JSON.
func Example() (string, error) {
	cfg := &Config{
		IP:             DefaultIP,
		UpdateInterval: new(defaultUpdateIntervalSeconds),
		LogLevel:       DefaultLogLevel,
		Cloudflare: []CloudflareConfig{
			{
				APIToken:   "your-cloudflare-api-token",
				ZoneName:   "example.com",
				RecordName: "ddns.example.com",
				Mode:       ModeUpsert,
				Type:       TypeAAndAAAA,
				TTL:        defaultTTLAuto,
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ") //nolint:gosec // serializes placeholder values, no real secrets
	if err != nil {
		return "", err
	}

	return string(data), nil
}
