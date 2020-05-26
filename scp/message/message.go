package message

import (
	"errors"
	hspb "github.com/StreamSpace/ss-light-client/scp/message/handshake"
	mppb "github.com/StreamSpace/ss-light-client/scp/message/micropayment"
	pb "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/protocol"
	msgio "github.com/libp2p/go-msgio"
	"io"
)

const (
	HandshakeProto    protocol.ID = "/scp/handshake/1.0.0"
	MicropaymentProto protocol.ID = "/scp/micropayment/1.0.0"
	SizeMax           int         = 32768
)

type ScpMsg interface {
	pb.Message
	ID() protocol.ID
	ToStream(io.Writer) error
}

func FromReader(p protocol.ID, r msgio.Reader) (ScpMsg, error) {
	msg, err := r.ReadMsg()
	if err != nil {
		return nil, err
	}

	var newMsg ScpMsg
	switch p {
	case HandshakeProto:
		newMsg = &hspb.HandshakeMsg{
			Credentials: &hspb.Credentials{},
		}
	case MicropaymentProto:
		newMsg = &mppb.MicropaymentMsg{
			SignedTxn: &mppb.SignedTxn{},
		}
	default:
		return nil, errors.New("Invalid message type")
	}

	err = pb.Unmarshal(msg, newMsg)
	r.ReleaseMsg(msg)
	if err != nil {
		return nil, err
	}

	return newMsg, nil
}
