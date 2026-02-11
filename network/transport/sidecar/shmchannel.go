package sidecar

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/linchenxuan/strix/log"
)

const _shmFileRight = 0o600

type shmChannel struct {
	readQueue  shmRingQueue
	writeQueue shmRingQueue
}

func (p *shmChannel) init(readShmFilePath string, writeShmFilePath string,
	timeout *time.Duration) error {
	defer func() {
		if !p.readQueue.inited() || !p.writeQueue.inited() {
			p.clean()
		}
	}()
	err := p.readQueue.init(readShmFilePath, timeout)
	if err != nil {
		return err
	}
	err = p.writeQueue.init(writeShmFilePath, timeout)
	if err != nil {
		return err
	}
	return nil
}

func (p *shmChannel) clean() {
	if err := p.readQueue.clean(); err != nil {
		log.Error().Err(err).Msg("Shm channel clean, read queue errs")
	}
	if err := p.writeQueue.clean(); err != nil {
		log.Error().Err(err).Msg("Shm channel clean, write queue errs")
	}
}

type shmHdrType uint8

const (
	_fake   shmHdrType = 5
	_normal shmHdrType = 7

	_skipMsgPerRead = 2000
)

type hdrType uint8

//nolint:varcheck
const (
	_protobuf hdrType = iota + 1
	_flatBuf
	_osHead
)

type shmMsgHead struct {
	// Validation bit, used to determine if the shared memory has been corrupted. Should be equal to SHM_VALIDATION_CODE.
	validation uint64
	// Version number. If the shared memory data structure needs to be upgraded in the future, this field can be used for message compatibility.
	version uint32
	// The time this message was written to shared memory. If the time exceeds the configured maximum time, the message will not be processed as it may no longer be meaningful.
	msgTime uint64
	// See ShmHeadType for details.
	shmHdrType shmHdrType
	// Currently all are PB messages, consider using flagBuf in the future.
	headType uint8
	// Message length. The total length of ShmMsgHead + ProtobufHead + Payload.
	msgLen uint32
	// ProtoBufHead header length.
	msgHeadLen uint32
}

// Decode unpacks the data.
func (s *shmMsgHead) Decode(buff []byte) error {
	if len(buff) < _shmHdrSize {
		return errors.New("ShmMsg DecodeBuff not enough")
	}
	offset := 0
	s.validation = *(*uint64)(unsafe.Pointer(&buff[offset]))
	offset += 8

	s.version = *(*uint32)(unsafe.Pointer(&buff[offset]))
	offset += 4

	s.msgTime = *(*uint64)(unsafe.Pointer(&buff[offset]))
	offset += 8

	s.shmHdrType = shmHdrType(buff[offset])
	offset++

	s.headType = buff[offset]
	offset++

	s.msgLen = *(*uint32)(unsafe.Pointer(&buff[offset]))
	offset += 4

	s.msgHeadLen = *(*uint32)(unsafe.Pointer(&buff[offset]))
	return nil
}

// Encode packs the data.
func (s *shmMsgHead) Encode(buff []byte) error {
	if len(buff) < _shmHdrSize {
		return errors.New("ShmMsg EncodeBuff not enough")
	}

	offset := 0
	*(*uint64)(unsafe.Pointer(&buff[offset])) = s.validation
	offset += 8

	*(*uint32)(unsafe.Pointer(&buff[offset])) = s.version
	offset += 4

	*(*uint64)(unsafe.Pointer(&buff[offset])) = s.msgTime
	offset += 8

	buff[offset] = uint8(s.shmHdrType)
	offset++

	buff[offset] = s.headType
	offset++

	*(*uint32)(unsafe.Pointer(&buff[offset])) = s.msgLen
	offset += 4

	*(*uint32)(unsafe.Pointer(&buff[offset])) = s.msgHeadLen
	return nil
}

type shmRingQueueStat struct {
	readDelayMaxMS   uint64
	readDelayTotalMS uint64
	readDelayCnt     uint64
	maxUsedShmSize   uint32
}

func (s *shmRingQueueStat) statUsedShmSize(i uint32) {
	maxSize := atomic.LoadUint32(&s.maxUsedShmSize)
	if maxSize < i {
		atomic.StoreUint32(&s.maxUsedShmSize, i)
	}
}

func (s *shmRingQueueStat) statReadDelay(delayMS uint64) {
	if delayMS < 0 {
		delayMS = 0
	}

	if s.readDelayMaxMS == 0 || s.readDelayMaxMS < delayMS {
		s.readDelayMaxMS = delayMS
	}
	s.readDelayCnt++
	s.readDelayTotalMS += delayMS
}

type shmRingQueue struct {
	mapped          bool
	shmMapFileData  []byte
	controlHeadData []byte
	msgData         []byte

	shmHeadEncodeBuf []byte
	shmFakeHeadBuf   []byte
	fakeShmMsg       shmMsgHead

	shmStat shmRingQueueStat
}

func (s *shmRingQueue) inited() bool {
	return s.mapped
}

func (s *shmRingQueue) init(shmFileName string, timeout *time.Duration) error {
	if s.mapped {
		return fmt.Errorf("shmfile:%s already mapped", shmFileName)
	}
	var err error
	err = errors.New("openfile timeout")
	var f *os.File
	for *timeout > 0 {
		f, err = os.OpenFile(shmFileName, os.O_RDWR, _shmFileRight)
		if err == nil {
			break
		}

		t := time.NewTimer(_waitFileSleepTime)
		*timeout -= _waitFileSleepTime
		<-t.C
	}

	if err != nil {
		return fmt.Errorf("open shmfile:%s, leftimeout:%d: %w", shmFileName, int64(*timeout), err)
	}

	defer func() {
		if f != nil {
			if ferr := f.Close(); ferr != nil {
				log.Error().Str("file", shmFileName).Err(err).Msg("close file")
			}
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("Stat shmfile:%s: %w", shmFileName, err)
	}

	size := fi.Size()
	if size <= 0 || size >= math.MaxUint32 {
		return fmt.Errorf("shmfile:%s size:%d <=0 || >= 4GB", shmFileName, size)
	}

	s.shmMapFileData, err = syscall.Mmap(int(f.Fd()), 0, int(size),
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("Mmap shmfile:%s: %w", shmFileName, err)
	}

	defer func() {
		if !s.mapped {
			if merr := syscall.Munmap(s.shmMapFileData); merr != nil {
				log.Error().Str("Munmap file", shmFileName).Err(err).End()
			}
		}
	}()

	if err = s.checkValid(); err != nil {
		return fmt.Errorf("checkValid shmfile:%s: %w", shmFileName, err)
	}

	s.fakeShmMsg = shmMsgHead{
		validation: _shmMsgValidCode,
		version:    _shmVersion,
		msgTime:    0,
		shmHdrType: _fake,
		headType:   uint8(_osHead),
		msgLen:     0,
		msgHeadLen: 0,
	}
	s.shmHeadEncodeBuf = make([]byte, _shmHdrSize)
	s.shmFakeHeadBuf = make([]byte, _shmHdrSize)
	s.controlHeadData = s.shmMapFileData[:_shmCtrlHdrSize]
	s.msgData = s.shmMapFileData[_shmCtrlHdrSize:]
	s.mapped = true
	return nil
}

func (s *shmRingQueue) checkValid() error {
	if len(s.shmMapFileData) < _shmCtrlHdrSize {
		return fmt.Errorf("shmMapFileData size:%d < _shmCtrlHdrSize:%d", len(s.shmMapFileData), _shmCtrlHdrSize)
	}
	if s.controlBuffSize() != _shmCtrlHdrSize {
		return fmt.Errorf("ControlHeadSize:%d != _shmCtrlHdrSize:%d", s.controlBuffSize(), _shmCtrlHdrSize)
	}
	if s.controlBuffSize()+s.queueBuffSize() != uint32(len(s.shmMapFileData)) {
		return fmt.Errorf("ControlHeadSize:%d + queuesize:%d != shmFileSize:%d", s.controlBuffSize(), s.queueBuffSize(), len(s.shmMapFileData))
	}
	if s.headIndex() > s.queueBuffSize() || s.tailIndex() > s.queueBuffSize() {
		return fmt.Errorf("IndexError: HeadIndex:%d > queueBuffSize:%d or tailIndex:%d > queueBuffSize:%d", s.headIndex(), s.queueBuffSize(), s.tailIndex(), s.queueBuffSize())
	}
	return nil
}

func (s *shmRingQueue) clean() error {
	if !s.mapped {
		return nil
	}

	err := syscall.Munmap(s.shmMapFileData)
	s.shmMapFileData = nil
	s.controlHeadData = nil
	s.msgData = nil
	s.mapped = false
	return err
}

func (s *shmRingQueue) hasData() bool {
	if !s.inited() {
		return false
	}
	return s.headIndex() != s.tailIndex()
}

// read reads data.
func (s *shmRingQueue) read(maxValidSec uint64) ([]byte, error) { //nolint:gocognit,gocyclo,revive,funlen
	if !s.inited() {
		return nil, errors.New("shmRingQueue not init")
	}
	var skippedMsgCnt int
	var maxUsedBuff uint32
	defer func() {
		if skippedMsgCnt > 0 { //nolint:revive,staticcheck
			// printer.Error().Int("Cnt", skippedMsgCnt).Msg("ShmQueue Skip ValidMsg Cnt")
		}
		s.shmStat.statUsedShmSize(maxUsedBuff)
	}()
	queueSize := s.queueBuffSize()
	nowMili := uint64(time.Now().UnixMilli())
	for {
		headIndex := s.headIndex()
		tailIndex := s.tailIndex()
		if headIndex > queueSize || tailIndex > queueSize {
			return nil, fmt.Errorf("shm control head invalid: headIndex:%d, tailIndex:%d, queueSize:%d", headIndex, tailIndex, queueSize)
		}
		// Check if the head is sufficient to read a complete ShmMsgHead header message.
		var iWaitReadBuffLen int64
		if tailIndex >= headIndex {
			iTailQueueSize := int64(tailIndex - headIndex)
			if iTailQueueSize == 0 {
				return nil, nil
			}
			if iTailQueueSize < _shmHdrSize {
				s.setHeadIndex(tailIndex)
				continue
			}
			iWaitReadBuffLen = int64(tailIndex - headIndex)
		} else {
			iTailQueueSize := int64(queueSize - headIndex)
			// Set the head pointer to the beginning of the queue.
			// By default, if there is not enough space for a _shmHdrSize, writing is skipped, so reading can also be skipped directly.
			if iTailQueueSize < _shmHdrSize {
				s.setHeadIndex(0)
				continue
			}
			iWaitReadBuffLen = int64(tailIndex - headIndex + queueSize)
		}
		if uint32(iWaitReadBuffLen) > maxUsedBuff {
			maxUsedBuff = uint32(iWaitReadBuffLen)
		}

		shmHead, err := s.checkReadMsgHead(iWaitReadBuffLen, headIndex, tailIndex)
		if err != nil {
			s.setHeadIndex(tailIndex)
			return nil, err
		}
		if shmHead.shmHdrType == _fake {
			s.setHeadIndex(0)
			continue
		}

		iMsgLen := int64(shmHead.msgLen)
		// If Callback is null, it means we want to drain the shared memory messages directly.
		// Validate the message insertion time (accurate to the second), and skip if the time exceeds the maximum configured valid time.
		if maxValidSec == 0 || shmHead.msgTime+maxValidSec*1000 >= nowMili {
			s.shmStat.statReadDelay(nowMili - shmHead.msgTime)

			if headIndex+uint32(iMsgLen) > uint32(len(s.msgData)) {
				s.setHeadIndex(tailIndex)
				return nil, fmt.Errorf("OnShmMsgRecv MsgIndex OverFlow: HeadIndex:%d, MsgLen:%d, quelen:%d", headIndex, iMsgLen, len(s.msgData))
			}
			// When entering here, the case HeadIndex >= tailIndex will never happen, because it has been guaranteed on the sending end.
			if _maxMsgSize < int(iMsgLen-_shmHdrSize) {
				return nil, fmt.Errorf("OnShmMsgRecv over: msgsize:%d, maxsize:%d", _maxMsgSize, int(iMsgLen-_shmHdrSize))
			}
			buff := make([]byte, (headIndex+uint32(iMsgLen))-(headIndex+_shmHdrSize))
			copy(buff, s.msgData[headIndex+_shmHdrSize:headIndex+uint32(iMsgLen)])
			s.setHeadIndex(s.headIndex() + uint32(iMsgLen))
			return buff[:iMsgLen-_shmHdrSize], nil
		}
		s.setHeadIndex(s.headIndex() + uint32(iMsgLen))
		skippedMsgCnt++
		if skippedMsgCnt <= _skipMsgPerRead {
			continue
		}
		return nil, nil
	}
}

func (s *shmRingQueue) checkReadMsgHead(
	iWaitReadBuffLen int64, headIndex uint32, tailIndex uint32) (shmHead *shmMsgHead, rerr error) {
	shmHead = &shmMsgHead{}
	if iWaitReadBuffLen <= 0 {
		return nil, fmt.Errorf("iWaitReadBuffLen:%d <= 0", iWaitReadBuffLen)
	}

	// Message correctness check
	if err := shmHead.Decode(s.msgData[headIndex : headIndex+_shmHdrSize]); err != nil {
		rerr = fmt.Errorf("ShmHead Decode: %w", err)
		return
	}
	if shmHead.validation != _shmMsgValidCode {
		rerr = fmt.Errorf("ShmHead validation:%d invalid", shmHead.validation)
		return
	}

	if _fake == shmHead.shmHdrType {
		return
	}

	// Message correctness check
	if _normal != shmHead.shmHdrType {
		rerr = fmt.Errorf("ShmHead shmHdrType:%d invalid", uint8(shmHead.shmHdrType))
		return
	}

	// Message correctness check
	iMsgLen := int64(shmHead.msgLen)
	if iWaitReadBuffLen < iMsgLen {
		rerr = fmt.Errorf("ShmHead MsgLenError: msgLen:%d, WaitToRecv:%d, Head:%d, Tail:%d, QueSize:%d", iMsgLen, iWaitReadBuffLen, headIndex, tailIndex, s.queueBuffSize())
		return
	}

	iBodyLen := iMsgLen - int64(shmHead.msgHeadLen) - _shmHdrSize
	if iBodyLen < 0 {
		rerr = fmt.Errorf("ShmHead MsgBodyLen Error: msgLen:%d, HeadLen:%d, WaitToRecv:%d, Head:%d, Tail:%d, QueSize:%d", iMsgLen, shmHead.msgHeadLen, iWaitReadBuffLen, headIndex, tailIndex, s.queueBuffSize())
	}
	return
}

func (s *shmRingQueue) mWrite(head []byte, bodies ...[]byte) error {
	uPBHeadSize := uint32(len(head))
	var uPayLoadSize int
	for _, body := range bodies {
		uPayLoadSize += len(body)
	}
	uMsgLen := uint32(uPayLoadSize) + uPBHeadSize + _shmHdrSize
	shmHead := shmMsgHead{
		// Validation bit, used to determine if the shared memory has been corrupted. Should be equal to SHM_VALIDATION_CODE.
		validation: _shmMsgValidCode,
		// Version number. If the shared memory data structure needs to be upgraded in the future, this field can be used for message compatibility.
		version: _shmVersion,
		// The time this message was written to shared memory. If the time exceeds the configured maximum time, the message will not be processed as it may no longer be meaningful.
		msgTime: uint64(time.Now().Unix()) * 1000,
		// See EShmHeadType for details.
		shmHdrType: _normal,
		// Currently all are PB messages, consider using flagBuf in the future.
		headType: uint8(_osHead),
		// Message length. The total length of ShmMsgHead + ProtobufHead + Payload.
		msgLen: uMsgLen,
		// ProtoBufHead header length.
		msgHeadLen: uPBHeadSize,
	}
	if err := shmHead.Encode(s.shmHeadEncodeBuf); err != nil {
		return err
	}

	uNewTailIndex, err := s.makeContiguousBuff(int64(uMsgLen))
	if err != nil {
		return err
	}

	// printer.Trace().UInt32("ShmMsgSize", uMsgLen).UInt32("newTail", uNewTailIndex+uMsgLen).
	// 	Int("Payload", uPayLoadSize).UInt32("head", uPBHeadSize).End()

	// Write ShmHead
	copy(s.msgData[uNewTailIndex:uNewTailIndex+_shmHdrSize], s.shmHeadEncodeBuf)
	uNewTailIndex += _shmHdrSize

	// Write PBHead
	copy(s.msgData[uNewTailIndex:uNewTailIndex+uPBHeadSize], head)
	uNewTailIndex += uPBHeadSize

	// Write PayLoad
	for _, body := range bodies {
		copy(s.msgData[uNewTailIndex:uNewTailIndex+uint32(len(body))], body)
		uNewTailIndex += uint32(len(body))
	}

	s.setTailIndex(uNewTailIndex)
	return nil
}

func (s *shmRingQueue) write(msgBuff []byte, uPBHeadSize uint32, uPayLoadSize uint32) error {
	if uPBHeadSize+uPayLoadSize == 0 || int(uPBHeadSize+uPayLoadSize) > len(msgBuff) {
		return fmt.Errorf("invalid size: head:%d, payload:%d, buff:%d", uPBHeadSize, uPayLoadSize, len(msgBuff))
	}

	uMsgLen := uPayLoadSize + uPBHeadSize + _shmHdrSize
	shmHead := shmMsgHead{
		// Validation bit, used to determine if the shared memory has been corrupted. Should be equal to SHM_VALIDATION_CODE.
		validation: _shmMsgValidCode,
		// Version number. If the shared memory data structure needs to be upgraded in the future, this field can be used for message compatibility.
		version: _shmVersion,
		// The time this message was written to shared memory. If the time exceeds the configured maximum time, the message will not be processed as it may no longer be meaningful.
		msgTime: uint64(time.Now().Unix()) * 1000,
		// See EShmHeadType for details.
		shmHdrType: _normal,
		// Currently all are PB messages, consider using flagBuf in the future.
		headType: uint8(_osHead),
		// Message length. The total length of ShmMsgHead + ProtobufHead + Payload.
		msgLen: uMsgLen,
		// ProtoBufHead header length.
		msgHeadLen: uPBHeadSize,
	}
	if err := shmHead.Encode(s.shmHeadEncodeBuf); err != nil {
		return err
	}

	uNewTailIndex, err := s.makeContiguousBuff(int64(uMsgLen))
	if err != nil {
		return err
	}

	// Write ShmHead
	copy(s.msgData[uNewTailIndex:uNewTailIndex+_shmHdrSize], s.shmHeadEncodeBuf)

	// Write PBHead+PayLoad
	copy(s.msgData[uNewTailIndex+_shmHdrSize:uNewTailIndex+uMsgLen], msgBuff)
	s.setTailIndex(uNewTailIndex + uMsgLen)

	return nil
}

//nolint:nestif
func (s *shmRingQueue) makeContiguousBuff(iMsgLen int64) (uint32, error) {
	var uNewTailindex uint32
	headIndex := int64(s.headIndex())
	tailIndex := int64(s.tailIndex())
	var usedSize int64
	defer func() {
		s.shmStat.statUsedShmSize(uint32(usedSize))
	}()
	if tailIndex >= headIndex {
		usedSize = tailIndex - headIndex
		// In order to avoid extra memory copy operations on the message receiving end, try to write the entire message into a contiguous space.
		iTailLeftSpace := int64(s.queueBuffSize()) - tailIndex
		if iTailLeftSpace < iMsgLen {
			iHeadLeftSpace := headIndex - 1
			if iMsgLen > iHeadLeftSpace {
				return uNewTailindex, fmt.Errorf("make ContiguousBuf Space Not Enough: msgLen:%d, HeadLeft:%d, TailLeft:%d", iMsgLen, iHeadLeftSpace, iTailLeftSpace)
			}

			// If there is not enough space to write the current message, but there is enough space to write a FakeMsg, then write a FakeMsg.
			if iTailLeftSpace >= _shmHdrSize {
				s.fakeShmMsg.msgLen = uint32(iTailLeftSpace)
				if err := s.fakeShmMsg.Encode(s.shmFakeHeadBuf); err != nil {
					return 0, err
				}
				copy(s.msgData[tailIndex:tailIndex+_shmHdrSize], s.shmFakeHeadBuf)
				// printer.Trace().Int64("tail", tailIndex).Int64("head", headIndex).
				// 	Int64("leftSapce", iTailLeftSpace).Msg("make ContiguourBuf write fake msg")
			}
			uNewTailindex = 0
			s.setTailIndex(0)
		} else {
			uNewTailindex = uint32(tailIndex)
		}
	} else {
		iLeftBuffSpace := headIndex - tailIndex - 1
		usedSize = int64(s.queueBuffSize()) - iLeftBuffSpace
		if iLeftBuffSpace < iMsgLen {
			return uNewTailindex, fmt.Errorf("make ContiguousBuf Not Enough2: msgLen:%d, leftSpace:%d", iMsgLen, iLeftBuffSpace)
		}
		uNewTailindex = uint32(tailIndex)
	}
	return uNewTailindex, nil
}

func (s *shmRingQueue) controlBuffSize() uint32 {
	return *(*uint32)(unsafe.Pointer(&s.shmMapFileData[0]))
}

func (s *shmRingQueue) queueBuffSize() uint32 {
	return *(*uint32)(unsafe.Pointer(&s.shmMapFileData[4]))
}

func (s *shmRingQueue) headIndex() uint32 {
	return atomic.LoadUint32((*uint32)(unsafe.Pointer(&s.shmMapFileData[8])))
}

func (s *shmRingQueue) setHeadIndex(index uint32) {
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&s.shmMapFileData[8])), index)
}

func (s *shmRingQueue) tailIndex() uint32 {
	return atomic.LoadUint32((*uint32)(unsafe.Pointer(&s.shmMapFileData[12])))
}

func (s *shmRingQueue) setTailIndex(index uint32) {
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&s.shmMapFileData[12])), index)
}
