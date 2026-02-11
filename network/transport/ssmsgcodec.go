package transport

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/linchenxuan/strix/network/codec"
	"github.com/linchenxuan/strix/network/pb"
)

// SSHeadDecodeResult .
type SSHeadDecodeResult struct {
	PreHead  *PreHead
	RouteHdr *pb.RouteHead
	PkgHead  *pb.PackageHead
	BodyData []byte
}

// DecodeSSMsgHead Decode SSMsg MsgPreHead + RouteHead + MsgPreHead + PkgHead + PkgBody.
func DecodeSSMsgHead(data []byte) (SSHeadDecodeResult, error) {
	var pkg SSHeadDecodeResult
	if len(data) < PRE_HEAD_SIZE {
		return pkg, fmt.Errorf("datalen:%d < PRE_HEAD_SIZE:%d", len(data), PRE_HEAD_SIZE)
	}

	h, err := DecodePreHead(data)
	if err != nil {
		return pkg, err
	}

	if h.HdrSize+h.BodySize+h.TailSize+PRE_HEAD_SIZE != uint32(len(data)) {
		return pkg, fmt.Errorf("headlen:%d bodyLen:%d tailLen:%d + PRE_HEAD_SIZE:%d != datalen:%d",
			h.HdrSize, h.BodySize, h.TailSize, PRE_HEAD_SIZE, len(data))
	}

	pkg.PreHead = h
	pkg.RouteHdr = &pb.RouteHead{}
	if err = codec.Decode(pkg.RouteHdr, data[PRE_HEAD_SIZE:PRE_HEAD_SIZE+h.HdrSize]); err != nil {
		return pkg, fmt.Errorf("headlen:%d bodyLen:%d + PRE_HEAD_SIZE:%d != datalen:%d, unmarshal routeHead err:%w",
			h.HdrSize, h.BodySize, PRE_HEAD_SIZE, len(data), err)
	}

	index := PRE_HEAD_SIZE + h.HdrSize
	h, err = DecodePreHead(data[index:])
	if err != nil {
		return pkg, fmt.Errorf("datalen:%d, unmarshal sub SSPreHead err:%w", len(data), err)
	}
	index += PRE_HEAD_SIZE
	if index+h.HdrSize+h.BodySize != uint32(len(data)) {
		return pkg, fmt.Errorf("index:%d + headlen:%d + bodylen:%d != datalen:%d",
			index, h.HdrSize, h.BodySize, len(data))
	}

	pkg.PkgHead = &pb.PackageHead{}
	if err = codec.Decode(pkg.PkgHead, data[index:index+h.HdrSize]); err != nil {
		return pkg, fmt.Errorf("headlen:%d bodyLen:%d + PRE_HEAD_SIZE:%d != datalen:%d, unmarshal pkgHead err:%w",
			h.HdrSize, h.BodySize, PRE_HEAD_SIZE, len(data), err)
	}
	index += h.HdrSize

	pkg.BodyData = data[index : index+h.BodySize]
	return pkg, nil
}

// DecodeSSMsgWithoutBody .
func DecodeSSMsgWithoutBody(data []byte, creator MsgCreator) (*TransRecvPkg, error) {
	if creator == nil {
		return nil, errors.New("nil MsgCreator")
	}
	result, err := DecodeSSMsgHead(data)
	if err != nil {
		return nil, err
	}
	if !creator.ContainsMsg(result.PkgHead.GetMsgID()) {
		return nil, errors.New("msgid not found: " + result.PkgHead.GetMsgID())
	}
	return NewTransRecvPkgWithBodyData(result.RouteHdr, result.PkgHead, result.BodyData, creator), nil
}

// DecodeSSMsg Decode SSMsg MsgPreHead + RouteHead + MsgPreHead + PkgHead + PkgBody.
func DecodeSSMsg(data []byte, creator MsgCreator) (*TransRecvPkg, error) {
	if creator == nil {
		return nil, errors.New("nil MsgCreator")
	}
	result, err := DecodeSSMsgHead(data)
	if err != nil {
		return nil, err
	}

	body, err := creator.CreateMsg(result.PkgHead.GetMsgID())
	if err != nil {
		return nil, err
	}
	if err := codec.Decode(body, result.BodyData); err != nil {
		return nil, err
	}

	return NewTransRecvPkgWithBody(result.RouteHdr, result.PkgHead, body), nil
}

// PackSSMsg pack all slice data to one.
func PackSSMsg(pkg TransSendPkg, buf *bytes.Buffer) error {
	if pkg.PkgHdr == nil {
		return errors.New("pkg missing hdr")
	}

	datas, err := EncodeSSMsg(pkg)
	if err != nil {
		return err
	}

	l := 0
	for i := 0; i < len(datas); i++ {
		l += len(datas[i])
	}
	buf.Grow(l)

	for i := 0; i < len(datas); i++ {
		if _, err := buf.Write(datas[i]); err != nil {
			return err
		}
	}

	return nil
}
