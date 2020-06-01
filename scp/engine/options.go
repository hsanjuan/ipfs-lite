package engine

import (
	mp "github.com/StreamSpace/ss-light-client/scp/message/micropayment"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"time"
)

// MessageSigner interface defines the Signing interface that is needed for
// SSBitswap to sign the transactions. This is provided by IPFS during init.
type MessageSigner interface {
	SignTxn(*mp.MicropaymentMsg) error
}

type dummyMessageSigner struct{}

func (d *dummyMessageSigner) SignTxn(msg *mp.MicropaymentMsg) error {
	msg.TxnHash = "dummySign"
	return nil
}

type SignatureVerifier interface {
	VerifyTxn(fromAddress string, msg *mp.MicropaymentMsg) bool
}

type dummySignatureVerifier struct{}

func (d *dummySignatureVerifier) VerifyTxn(_ string, _ *mp.MicropaymentMsg) bool {
	return true
}

// WhitelistChecker is the interface required to check if peer is whitelisted or
// not. Different mechanisims can be used to do this check. IPFS will provide this
// during init.
type WhitelistChecker interface {
	IsWhitelisted(address string) bool
}

type dummyWhitelistChecker struct{}

func (d *dummyWhitelistChecker) IsWhitelisted(_ string) bool {
	return true
}

type SSConfig interface {
	String() string
	UserId() peer.ID
	DeviceId() string
	Role() string
	Epoch() time.Time
	Cycle() time.Duration
	Rate() float64
	RawMetadata() map[string]interface{}
}

type dummyConf struct{}

const INVALID string = "dummy"

func (d *dummyConf) String() string { return INVALID }

func (d *dummyConf) UserId() peer.ID { return "dummy" }

func (d *dummyConf) DeviceId() string { return "dummy" }

func (d *dummyConf) Role() string { return INVALID }

func (d *dummyConf) Epoch() time.Time { return time.Now() }

func (d *dummyConf) Cycle() time.Duration { return time.Second }

func (d *dummyConf) Rate() float64 { return 0 }

func (d *dummyConf) RawMetadata() map[string]interface{} { return nil }

type Option func(*Engine)

func WithWhitelistChecker(wc WhitelistChecker) Option {
	return func(e *Engine) {
		e.wlChecker = wc
	}
}

func WithMsgSigner(ms MessageSigner) Option {
	return func(e *Engine) {
		e.msgSigner = ms
	}
}

func WithSignVerifier(sv SignatureVerifier) Option {
	return func(e *Engine) {
		e.signVerifier = sv
	}
}

func WithSSConfig(conf SSConfig) Option {
	return func(e *Engine) {
		e.ssConf = conf
	}
}

func WithLedgerStore(conf SSConfig) Option {
	return func(e *Engine) {
		e.ssConf = conf
	}
}
