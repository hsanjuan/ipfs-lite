package ssCrypto

import (
	b64 "encoding/base64"
	"encoding/json"
	"errors"

	mp "github.com/StreamSpace/ss-light-client/scp/message/micropayment"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("ssCrypto")

type ipfsSigner struct {
	host.Host
}

func New(h host.Host) *ipfsSigner {
	return &ipfsSigner{
		Host: h,
	}
}

func createTxnBuffer(msg *mp.MicropaymentMsg) ([]byte, error) {
	txn := make(map[string]interface{})
	txn["to"] = msg.Receiver
	txn["bcn"] = msg.BillingCycle
	txn["amount"] = msg.Amount
	if msg.Mtdt != nil {
		for k, v := range msg.Mtdt.Vals {
			if v.IncludeSignature {
				switch v.Val.(type) {
				case *mp.Metadata_MtdtVal_IntVal:
					txn[k] = v.GetIntVal()
				case *mp.Metadata_MtdtVal_StrVal:
					txn[k] = v.GetStrVal()
				default:
					return nil, errors.New("Unknown metadata type")
				}
			}
		}
	}
	return json.Marshal(txn)
}

func (i *ipfsSigner) SignTxn(msg *mp.MicropaymentMsg) error {
	buf, err := createTxnBuffer(msg)
	if err != nil {
		log.Errorf("Failed to marshal msg while signing Err:%s", err.Error())
		return err
	}
	key := i.Peerstore().PrivKey(i.ID())
	if key == nil {
		log.Errorf("Private key missing for node %s", i.ID().Pretty())
		return errors.New("Node private key missing")
	}
	sign, err := key.Sign(buf)
	if err != nil {
		log.Errorf("Failed signing Err:%s", err.Error())
		return err
	}
	msg.TxnHash = b64.StdEncoding.EncodeToString(sign)
	return nil
}

func (i *ipfsSigner) VerifyTxn(fromAddress string, msg *mp.MicropaymentMsg) bool {
	buf, err := createTxnBuffer(msg)
	if err != nil {
		log.Errorf("Failed to marshal msg while verifying Err:%s", err.Error())
		return false
	}
	// Currently we just transform the peer.ID to string while sending the message
	// inversely, we are just casting it back.
	pId := peer.ID(fromAddress)

	key := i.Peerstore().PubKey(pId)
	if key == nil {
		log.Errorf("Public key missing for peer %s", pId.Pretty())
		return false
	}
	signBuf, err := b64.StdEncoding.DecodeString(msg.TxnHash)
	if err != nil {
		log.Errorf("Failed decoding base64 %s", err.Error())
		return false
	}
	res, err := key.Verify(buf, signBuf)
	if err != nil {
		log.Errorf("Failed verifying public key Err:%s", err.Error())
		return false
	}
	return res
}
