package sidecar

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"github.com/linchenxuan/strix/network/pb"
)

type unixHead struct {
	msgID int32
	// Length of the PB header
	headSize int32
	// unixHead length + PB header length + PayLoad length
	msgSize int32
	// msgFlag
	msgFlag int32
}

// Decode head decode func.
func (p *unixHead) Decode(buff []byte) error {
	if len(buff) < _unixSockHdrSize {
		return errors.New("buff not enough")
	}

	p.msgID = *(*int32)(unsafe.Pointer(&buff[0]))
	p.headSize = *(*int32)(unsafe.Pointer(&buff[4]))
	p.msgSize = *(*int32)(unsafe.Pointer(&buff[8]))
	p.msgFlag = *(*int32)(unsafe.Pointer(&buff[12]))

	if p.msgID <= int32(pb.EPBMsgType_PBMT_MIN) || p.msgID >= int32(pb.EPBMsgType_PBMT_MAX) {
		return fmt.Errorf("msgID:%d invalid", p.msgID)
	}

	if p.msgID == 0 || p.msgSize > _maxMsgSize || p.msgSize < p.headSize+_unixSockHdrSize {
		return fmt.Errorf("msgID:%d msgSize:%d headsize:%d invalid", p.msgID, p.msgSize, p.headSize)
	}
	return nil
}

// Encode head encode func.
func (p *unixHead) Encode(buff []byte) bool {
	if len(buff) < _unixSockHdrSize {
		return false
	}
	*(*int32)(unsafe.Pointer(&buff[0])) = p.msgID
	*(*int32)(unsafe.Pointer(&buff[4])) = p.headSize
	*(*int32)(unsafe.Pointer(&buff[8])) = p.msgSize
	*(*int32)(unsafe.Pointer(&buff[12])) = p.msgFlag
	return true
}

func writeFull(r *net.UnixConn, buf []byte) (err error) {
	n := 0
	lenth := len(buf)
	for n < lenth && err == nil {
		var nn int
		nn, err = r.Write(buf[n:])
		n += nn
	}
	if n >= lenth {
		err = nil
	}
	return
}

func readFull(r *net.UnixConn, buf []byte) (err error) {
	n := 0
	lenth := len(buf)
	for n < lenth && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	if n >= lenth {
		err = nil
	}
	return
}
