package dispatcher

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/linchenxuan/strix/network/handler"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type testMsgLayerReceiver struct {
	t            *testing.T
	err          error
	sendBackFunc func(*transport.TransSendPkg) error
	called       int
}

var _ handler.MsgLayerReceiver = (*testMsgLayerReceiver)(nil)

func (r *testMsgLayerReceiver) OnRecvDispatcherPkg(delivery handler.Delivery) error {
	r.called++
	if r.err != nil {
		return r.err
	}

	dd, ok := delivery.(*DispatcherDelivery)
	if !ok {
		r.t.Fatalf("delivery type mismatch: got %T", delivery)
	}
	if dd.GetPkgHdr() == nil {
		r.t.Fatalf("expected package header")
	}
	if dd.GetProtoInfo() == nil {
		r.t.Fatalf("expected proto info")
	}
	if dd.GetPkgHdr().GetMsgID() != dd.GetProtoInfo().GetMsgID() {
		r.t.Fatalf("msg id mismatch: pkg=%q proto=%q", dd.GetPkgHdr().GetMsgID(), dd.GetProtoInfo().GetMsgID())
	}

	if r.sendBackFunc != nil {
		if dd.TransSendBack == nil {
			r.t.Fatalf("expected transport send-back callback")
		}
		if err := dd.TransSendBack(&transport.TransSendPkg{
			PkgHdr: &pb.PackageHead{MsgID: dd.GetPkgHdr().GetMsgID()},
			Body:   &emptypb.Empty{},
		}); err != nil {
			return err
		}
	}
	return nil
}

func TestDispatcherOnRecvTransportPkg_SuccessEndToEnd(t *testing.T) {
	msgID := testMsgID("dispatcher_success")
	registerTestMsgProto(msgID, message.MsgLayerType_Stateless)

	d, err := NewDispatcher(defaultTestDispatcherCfg(), nil)
	if err != nil {
		t.Fatalf("NewDispatcher failed: %v", err)
	}

	sendBackCalled := 0
	receiver := &testMsgLayerReceiver{
		t: t,
		sendBackFunc: func(pkg *transport.TransSendPkg) error {
			sendBackCalled++
			if pkg == nil || pkg.PkgHdr == nil {
				return errors.New("nil send pkg")
			}
			if pkg.PkgHdr.GetMsgID() != msgID {
				return fmt.Errorf("unexpected send msg id: %s", pkg.PkgHdr.GetMsgID())
			}
			return nil
		},
	}
	if err := d.RegisterMsglayer(message.MsgLayerType_Stateless, receiver); err != nil {
		t.Fatalf("RegisterMsglayer failed: %v", err)
	}

	td := &transport.TransportDelivery{
		TransSendBack: receiver.sendBackFunc,
		Pkg: &transport.TransRecvPkg{
			PkgHdr: &pb.PackageHead{MsgID: msgID},
		},
	}

	if err := d.OnRecvTransportPkg(td); err != nil {
		t.Fatalf("OnRecvTransportPkg failed: %v", err)
	}
	if receiver.called != 1 {
		t.Fatalf("receiver call count mismatch: got %d want 1", receiver.called)
	}
	if sendBackCalled != 1 {
		t.Fatalf("sendBack call count mismatch: got %d want 1", sendBackCalled)
	}
}

func TestDispatcherOnRecvTransportPkg_ProtoMissing(t *testing.T) {
	msgID := testMsgID("dispatcher_missing_proto")

	d, err := NewDispatcher(defaultTestDispatcherCfg(), nil)
	if err != nil {
		t.Fatalf("NewDispatcher failed: %v", err)
	}

	err = d.OnRecvTransportPkg(&transport.TransportDelivery{
		Pkg: &transport.TransRecvPkg{
			PkgHdr: &pb.PackageHead{MsgID: msgID},
		},
	})
	if err == nil {
		t.Fatalf("expected error for missing proto info")
	}
	if !strings.Contains(err.Error(), "proto info not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatcherOnRecvTransportPkg_HandlerError(t *testing.T) {
	msgID := testMsgID("dispatcher_handler_error")
	registerTestMsgProto(msgID, message.MsgLayerType_Stateless)

	d, err := NewDispatcher(defaultTestDispatcherCfg(), nil)
	if err != nil {
		t.Fatalf("NewDispatcher failed: %v", err)
	}

	expectedErr := errors.New("handler failed")
	receiver := &testMsgLayerReceiver{t: t, err: expectedErr}
	if err := d.RegisterMsglayer(message.MsgLayerType_Stateless, receiver); err != nil {
		t.Fatalf("RegisterMsglayer failed: %v", err)
	}

	err = d.OnRecvTransportPkg(&transport.TransportDelivery{
		Pkg: &transport.TransRecvPkg{
			PkgHdr: &pb.PackageHead{MsgID: msgID},
		},
	})
	if err == nil {
		t.Fatalf("expected handler error")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("error mismatch: got %v want %v", err, expectedErr)
	}
}

func TestDispatcherOnRecvTransportPkg_NilInput(t *testing.T) {
	d, err := NewDispatcher(defaultTestDispatcherCfg(), nil)
	if err != nil {
		t.Fatalf("NewDispatcher failed: %v", err)
	}

	cases := []struct {
		name string
		td   *transport.TransportDelivery
	}{
		{name: "nil delivery", td: nil},
		{name: "nil package", td: &transport.TransportDelivery{}},
		{name: "nil package header", td: &transport.TransportDelivery{Pkg: &transport.TransRecvPkg{}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := d.OnRecvTransportPkg(tc.td)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), "nil transport delivery or package header") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func defaultTestDispatcherCfg() *DispatcherConfig {
	return &DispatcherConfig{
		RecvRateLimit: 10000,
		TokenBurst:    1000,
	}
}

func registerTestMsgProto(msgID string, layer message.MsgLayerType) {
	message.RegisterMsgInfo(&message.MsgProtoInfo{
		New:          func() proto.Message { return &emptypb.Empty{} },
		MsgID:        msgID,
		MsgReqType:   message.MRTNtf,
		IsCS:         true,
		MsgLayerType: layer,
	})
	message.RegisterMsgHandle(msgID, struct{}{}, layer)
}

func testMsgID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
