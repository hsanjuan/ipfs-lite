package micropayment

import (
	"encoding/binary"
	"encoding/json"
	pb "github.com/golang/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/protocol"
	"io"
)

//go:generate protoc --proto_path=. --go_out=. micropayment.proto

type MicropaymentMsg struct {
	*SignedTxn
}

func NewMicroPayment(
	amount float64,
	billingCycle int64,
	toAddr string,
	mtdt map[string]interface{},
) *MicropaymentMsg {
	return &MicropaymentMsg{
		SignedTxn: &SignedTxn{
			Receiver:     toAddr,
			Amount:       amount,
			BillingCycle: int32(billingCycle),
			Mtdt:         fromRawMetadata(mtdt),
		},
	}
}

func (h *MicropaymentMsg) ID() protocol.ID {
	return protocol.ID("/scp/micropayment/1.0.0")
}

func (h *MicropaymentMsg) ToStream(w io.Writer) error {
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

func fromRawMetadata(mtdt map[string]interface{}) *Metadata {
	if mtdt == nil || len(mtdt) == 0 {
		return nil
	}
	m := new(Metadata)
	m.Vals = make(map[string]*Metadata_MtdtVal)
	for k, v := range mtdt {
		switch v.(type) {
		case int:
			m.Vals[k] = &Metadata_MtdtVal{
				Val: &Metadata_MtdtVal_IntVal{
					IntVal: int32(v.(int)),
				},
			}
		case string:
			m.Vals[k] = &Metadata_MtdtVal{
				Val: &Metadata_MtdtVal_StrVal{
					StrVal: v.(string),
				},
			}
		}
		if k == "download_index" {
			m.Vals[k].IncludeSignature = true
		}
	}
	return m
}

// Helper to convert proto metadata to generic format to use in json
func (h *MicropaymentMsg) RawMetadataBytes() []byte {
	if h.SignedTxn.Mtdt != nil {
		rawMtdt := make(map[string]interface{})
		for k, v := range h.SignedTxn.Mtdt.Vals {
			switch v.Val.(type) {
			case *Metadata_MtdtVal_IntVal:
				rawMtdt[k] = v.GetIntVal()
			case *Metadata_MtdtVal_StrVal:
				rawMtdt[k] = v.GetStrVal()
			}
		}
		buf, err := json.Marshal(rawMtdt)
		if err != nil {
			return nil
		}
		return buf
	}
	return nil
}
