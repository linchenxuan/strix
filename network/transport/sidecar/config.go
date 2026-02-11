package sidecar

import (
	"path"
	"strconv"
)

const (
	_lockFileName   = "client.lock"
	_unixSockHBFreq = 3
	_minUnixHB      = 500
)

type SidecarConfig struct {
	Tag                     string `json:"Tag"                     mapstructure:"Tag"`
	ShmDirPath              string `json:"ShmDirPath"              mapstructure:"ShmDirPath"`
	ShmRecvDataFileName     string `json:"ShmRecvDataFileName"     mapstructure:"ShmRecvDataFileName"`
	ShmSendDataFileName     string `json:"ShmSendDataFileName"     mapstructure:"ShmSendDataFileName"`
	ShmSendQueueSize        int    `json:"ShmSendQueueSize"        mapstructure:"ShmSendQueueSize"`
	ShmRecvQueueSize        int    `json:"ShmRecvQueueSize"        mapstructure:"ShmRecvQueueSize"`
	UnixSocketName          string `json:"UnixSocketName"          mapstructure:"UnixSocketName"`
	UnixSockHBTimeoutMS     int    `json:"UnixSockHBTimeoutMS"     mapstructure:"UnixSockHBTimeoutMS"`
	UnixSockRegistTimeoutMS int    `json:"UnixSockRegistTimeoutMS" mapstructure:"UnixSockRegistTimeoutMS"`
	UnixSockRetryMS         int    `json:"UnixSockRetryMS"         mapstructure:"UnixSockRetryMS"`
	MeshCltValidMsgSecs     int    `json:"MeshCltValidMsgSecs"     mapstructure:"MeshCltValidMsgSecs"`
	ShmpipeCnt              int    `json:"ShmpipeCnt"              mapstructure:"ShmpipeCnt"`
	LockFileName            string `json:"LockFileName"              mapstructure:"LockFileName,default:client.lock"`
	UnixSockHBFreq          int    `json:"UnixSockHBFreq"              mapstructure:"UnixSockHBFreq,default:3"`
	MinUnixHB               int    `json:"MinUnixHB"              mapstructure:"MinUnixHB,default:500"`
}

func (c *SidecarConfig) getRecvDataFilePath(idx int) string {
	if idx != 0 {
		return path.Join(c.ShmDirPath, c.ShmRecvDataFileName+strconv.Itoa(idx))
	}
	return path.Join(c.ShmDirPath, c.ShmRecvDataFileName)
}

func (c *SidecarConfig) getSendDataFilePath(idx int) string {
	if idx != 0 {
		return path.Join(c.ShmDirPath, c.ShmSendDataFileName+strconv.Itoa(idx))
	}
	return path.Join(c.ShmDirPath, c.ShmSendDataFileName)
}

func (c *SidecarConfig) getUnixSockPath() string {
	return path.Join(c.ShmDirPath, c.UnixSocketName)
}

func (c *SidecarConfig) getFileLockName() string {
	return _lockFileName
}

func (c *SidecarConfig) getShmpipeCnt() int {
	return c.ShmpipeCnt
}

func (c *SidecarConfig) getShmDirPath() string {
	return c.ShmDirPath
}

func (c *SidecarConfig) getClientFileLockPath() string {
	return path.Join(c.ShmDirPath, _lockFileName)
}

func (c *SidecarConfig) getUnixHBTimeMS() int {
	ms := c.UnixSockHBTimeoutMS / _unixSockHBFreq
	if ms < _minUnixHB {
		return _minUnixHB
	}
	return ms
}
