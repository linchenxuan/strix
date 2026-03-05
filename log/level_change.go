package log

// LevelChangeEntry 定义单条动态日志级别覆盖规则。
type LevelChangeEntry struct {
	// FileName 为目标源码文件路径。
	FileName string

	// LineNum 为目标行号；0 表示文件内所有行。
	LineNum int

	// LogLevel 为覆盖后的日志级别。
	LogLevel int
}

// levelChange 管理按文件/行号的日志级别覆盖。
type levelChange struct {
	changes map[string]map[int]Level
}

// newLevelChange 从配置构建覆盖表。
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

// AddChange 添加或覆盖一条规则。
func (lc *levelChange) AddChange(entry LevelChangeEntry) {
	if _, ok := lc.changes[entry.FileName]; !ok {
		lc.changes[entry.FileName] = make(map[int]Level)
	}
	lc.changes[entry.FileName][entry.LineNum] = Level(entry.LogLevel)
}

// GetLevel 返回指定位置的有效日志级别。
// 若无命中规则，返回传入级别。
func (lc *levelChange) GetLevel(fileName string, lineNum int, level Level) Level {
	if _, ok := lc.changes[fileName]; !ok {
		return level
	}
	if lv, ok := lc.changes[fileName][lineNum]; ok {
		return lv
	}
	return level
}
