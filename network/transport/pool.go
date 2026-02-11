package transport

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/utils/pool"
)

// _encodeBufPool bytes.Buffer pool.
var _encodeBufPool = pool.NewPool("encodebufpool", func() any {
	return &bytes.Buffer{}
})

// GetEncodeBuf gets a buffer for encoding.
func GetEncodeBuf() *bytes.Buffer {
	buf, ok := _encodeBufPool.Get().(*bytes.Buffer)
	if !ok {
		log.Error().Msg("_encodeBufPool type assert *bytes.Buffer")
		return &bytes.Buffer{}
	}
	return buf
}

// PutEncodeBuf returns a buffer to the pool.
func PutEncodeBuf(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	buf.Reset()
	_encodeBufPool.Put(buf)
}

const _EncodeBufferSize = 5

// EncoderBuffer .
type EncoderBuffer struct {
	Datas [][]byte
}

// EncodePreHead .
func (buf *EncoderBuffer) EncodePreHead(idx int, preHead PreHead) {
	data := buf.Datas[idx]
	if len(data) < PRE_HEAD_SIZE {
		data = make([]byte, PRE_HEAD_SIZE)
		buf.Datas[idx] = data
	} else if len(data) > PRE_HEAD_SIZE {
		data = data[:PRE_HEAD_SIZE]
		buf.Datas[idx] = data
	}
	binary.LittleEndian.PutUint32(data[0:4], preHead.HdrSize)
	binary.LittleEndian.PutUint32(data[4:8], preHead.BodySize)
	binary.LittleEndian.PutUint32(data[8:12], preHead.TailSize)
}

// Length .
func (buf *EncoderBuffer) Length() int {
	var msgSize int
	for i := 0; i < len(buf.Datas); i++ {
		msgSize += len(buf.Datas[i])
	}
	return msgSize
}

// EncoderPool optimize for cs pkg encode mem use.
type EncoderPool struct {
	*sync.Pool
}

// NewEncoderPool create pool encoder.
func NewEncoderPool() *EncoderPool {
	return &EncoderPool{
		Pool: &sync.Pool{
			New: func() any {
				return &EncoderBuffer{
					Datas: make([][]byte, _EncodeBufferSize),
				}
			},
		},
	}
}

// Put release alloced data.
func (en *EncoderPool) Put(buf *EncoderBuffer) {
	if len(buf.Datas) != _EncodeBufferSize {
		log.Error().Int("len", len(buf.Datas)).Msg("Not in CSPoolEncoder put")
		return
	}
	en.Pool.Put(buf)
}

// Get get data from pool data.
func (en *EncoderPool) Get() *EncoderBuffer {
	buf, _ := en.Pool.Get().(*EncoderBuffer) //nolint:revive
	datas := buf.Datas
	for i := 0; i < len(buf.Datas); i++ {
		if len(datas[i]) > 0 {
			datas[i] = datas[i][:0]
		}
	}
	return buf
}
