package handshake

import (
	"encoding/binary"
	pb "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/protocol"
	"io"
)

//go:generate protoc --proto_path=. --go_out=. handshake.proto

type HandshakeMsg struct {
	*Credentials
}

func NewHandshake(role, deviceID string) *HandshakeMsg {
	return &HandshakeMsg{
		Credentials: &Credentials{
			Role:     role,
			DeviceId: deviceID,
		},
	}
}

func (h *HandshakeMsg) ID() protocol.ID {
	return protocol.ID("/scp/handshake/1.0.0")
}

func (h *HandshakeMsg) ToStream(w io.Writer) error {
	size := pb.Size(h)
	sizeBytes := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(sizeBytes, uint64(size))
	msgBytes, err := pb.Marshal(h)
	if err != nil {
		return err
	}
	buf := append(sizeBytes[:n], msgBytes...)

	_, err = w.Write(buf)
	return err
}
