package main

import (
	"strings"
	"testing"
)

func TestBanner(t *testing.T) {
	b := banner("v9.9.9-test")

	if strings.HasPrefix(b, "\n") {
		t.Error("banner() keeps the leading newline — the artwork trim is gone")
	}
	if !strings.Contains(b, "v9.9.9-test") {
		t.Errorf("banner() = %q, want the substituted version", b)
	}
	if strings.Contains(b, "{version}") {
		t.Errorf("banner() = %q, want the {version} placeholder replaced", b)
	}
	if !strings.Contains(b, "ZJDDNS") || !strings.Contains(b, "https://github.com/hezhijie0327/ZJDDNS") {
		t.Errorf("banner() = %q, want the artwork body intact", b)
	}
}

func TestGetVersion(t *testing.T) {
	// Save and restore the ldflags-populated fields; tests mutate them.
	commit, buildTime := CommitHash, BuildTime
	t.Cleanup(func() { CommitHash, BuildTime = commit, buildTime })

	t.Run("base without build metadata", func(t *testing.T) {
		CommitHash, BuildTime = "", ""

		got := getVersion()
		if !strings.HasPrefix(got, "v"+Version+" (") {
			t.Errorf("getVersion() = %q, want the v<version> (go) base form", got)
		}
	})

	t.Run("both metadata fields surface", func(t *testing.T) {
		CommitHash, BuildTime = "abc1234", "2026-11-14T07:00:00"

		got := getVersion()
		if !strings.Contains(got, "-abc1234@2026-11-14T07:00:00") {
			t.Errorf("getVersion() = %q, want the v<ver>-<commit>@<time> form", got)
		}
	})

	t.Run("single metadata field still surfaces", func(t *testing.T) {
		CommitHash, BuildTime = "abc1234", ""

		got := getVersion()
		if !strings.Contains(got, "-abc1234") {
			t.Errorf("getVersion() = %q, want the lone commit surfaced", got)
		}
	})
}
