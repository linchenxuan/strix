package log

import "strconv"

var _UnknownCallerInfo = &callerInfo{
	file:     "unknown",
	function: "unknown",
}

type callerInfo struct {
	file     string
	function string
	line     int
	info     string
}

func newCallerInfo(file string, function string, line int) *callerInfo {
	return &callerInfo{
		file:     file,
		function: function,
		line:     line,
		info:     file + ":" + strconv.Itoa(line) + " " + function,
	}
}

func (c *callerInfo) String() string {
	return c.info
}
