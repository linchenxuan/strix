package event

// resetManagerForTest 重置默认事件管理器，仅供测试使用。
func resetManagerForTest() {
	defaultManager = newManager()
}
