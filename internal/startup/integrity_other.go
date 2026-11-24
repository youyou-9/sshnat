//go:build !windows

package startup

// IntegrityLevel reports LevelMedium on platforms that have no mandatory
// integrity levels, so the desktop UI is always allowed to start.
func IntegrityLevel() (Level, error) { return LevelMedium, nil }
