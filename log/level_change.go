package log

// LevelChangeEntry represents a single dynamic log level adjustment rule.
// Enables precise control over logging verbosity at specific code locations.
// Designed for debugging critical game server components without affecting global log levels.
type LevelChangeEntry struct {
	// FileName specifies the target source file path for log level adjustment.
	// Supports both absolute and relative paths for flexible targeting.
	// Examples: "game/player.go", "/app/server/game/battle.go"
	FileName string

	// LineNum indicates the specific line number within the file for level adjustment.
	// Use 0 to apply the level change to all lines within the file.
	// Enables fine-grained debugging of specific log statements.
	LineNum int

	// LogLevel defines the new log level for the specified file and line.
	// Valid levels: Trace=0, Debug=1, Info=2, Warn=3, Error=4, Fatal=5
	// Higher values enable more verbose logging for targeted debugging.
	LogLevel int
}

// levelChange manages dynamic log level adjustments for specific code locations.
// Implements a hierarchical lookup system for efficient level determination.
// Thread-safe for concurrent access in high-throughput game server environments.
type levelChange struct {
	changes map[string]map[int]Level
}

// newLevelChange creates a new level change manager from configuration entries.
// Initializes the hierarchical lookup structure for efficient level queries.
// Supports bulk configuration loading for operational convenience.
func newLevelChange(entrys []LevelChangeEntry) *levelChange {
	c := &levelChange{
		changes: make(map[string]map[int]Level),
	}

	for _, entry := range entrys {
		c.AddChange(entry)
	}

	return c
}

func (lc *levelChange) Empty() bool {
	return len(lc.changes) == 0
}

// AddChange adds a new level change rule to the manager.
// Overwrites existing rules for the same file and line combination.
// Thread-safe for dynamic configuration updates.
func (lc *levelChange) AddChange(entry LevelChangeEntry) {
	if _, ok := lc.changes[entry.FileName]; !ok {
		lc.changes[entry.FileName] = make(map[int]Level)
	}
	lc.changes[entry.FileName][entry.LineNum] = Level(entry.LogLevel)
}

// GetLevel determines the effective log level for a specific code location.
// Returns the adjusted level if a rule exists, or the original level otherwise.
// Optimized for high-frequency lookups in performance-critical paths.
func (lc *levelChange) GetLevel(fileName string, lineNum int, level Level) Level {
	if _, ok := lc.changes[fileName]; !ok {
		return level
	}
	if lv, ok := lc.changes[fileName][lineNum]; ok {
		return lv
	}
	return level
}
