package ss_ledger

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	pb "github.com/golang/protobuf/proto"
	logging "github.com/ipfs/go-log/v2"
)

//go:generate protoc --proto_path=. --go_out=. ss_ledger.proto

var log = logging.Logger("ss_ledger")

func prettyPrintTxn(txnHash string) string {
	if len(txnHash) == 0 {
		return "0x00000.....00000"
	}

	return "0x" + txnHash[:5] + "....." + txnHash[len(txnHash)-5:]
}

func (s *SSLedger) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"Partner":            s.Partner,
		"PartnerDevice":      s.DeviceId,
		"Role":               s.Role,
		"Exchanges":          s.MpExchangeCount,
		"Whitelisted":        s.Whitelisted,
		"Invoice":            s.Invoice,
		"Paid":               s.Sent,
		"Paid Bytes":         s.BytesPaid,
		"Received Bytes":     s.BytesRecvd,
		"Received Blocks":    s.BlocksRecvd,
		"Received":           s.Recvd,
		"Bytes pay received": s.BytesPayRecvd,
		"Sent Bytes":         s.BytesSent,
		"Sent Blocks":        s.BlocksSent,
		"Signed Txn":         prettyPrintTxn(s.SignedMp),
	}
}

// Store : Functions supported by SS Ledger Store
type Store interface {
	Get(*SSLedger) error
	Store(*SSLedger) error
	List() ([]*SSLedger, error)
	Update(int64) error
	BillingCycle() int64
	GetPending() (map[int][]*SSLedger, error)
	ClearPending([]int) ([]int, error)
	Close() error
}

type DummyStore struct{}

func (s *DummyStore) Get(val *SSLedger) error {
	return nil
}

func (s *DummyStore) Store(val *SSLedger) error {
	return nil
}

func (s *DummyStore) List() ([]*SSLedger, error) {
	return []*SSLedger{
		&SSLedger{},
	}, nil
}

func (s *DummyStore) Update(_ int64) error {
	return nil
}

func (s *DummyStore) BillingCycle() int64 {
	return 0
}

func (s *DummyStore) GetPending() (map[int][]*SSLedger, error) {
	return map[int][]*SSLedger{}, nil
}

func (s *DummyStore) ClearPending(res []int) ([]int, error) {
	return res, nil
}

func (s *DummyStore) Close() error {
	return nil
}

type ssLedgerStore struct {
	currCycle int64
	cycleMtx  sync.Mutex
	ledgers   sync.Map
}

func NewMapLedgerStore() Store {
	return &ssLedgerStore{
		currCycle: 1,
	}
}

func (s *ssLedgerStore) getKey(p string) string {
	return s.prefix(s.currCycle) + p
}

func (s *ssLedgerStore) prefix(cycle int64) string {
	return fmt.Sprintf("%d_", s.currCycle)
}

func copyLedger(a, b *SSLedger) error {
	buf, err := pb.Marshal(a)
	if err != nil {
		return err
	}
	return pb.Unmarshal(buf, b)
}

func (s *ssLedgerStore) Get(val *SSLedger) error {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()

	mVal, ok := s.ledgers.Load(s.getKey(val.Partner))
	if !ok {
		return errors.New("Key not found")
	}
	pVal, ok := mVal.(*SSLedger)
	if !ok {
		return errors.New("Invalid type")
	}
	return copyLedger(pVal, val)
}

func (s *ssLedgerStore) Store(val *SSLedger) error {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()
	l := new(SSLedger)
	copyLedger(val, l)
	s.ledgers.Store(s.getKey(val.Partner), l)
	return nil
}

func (s *ssLedgerStore) List() ([]*SSLedger, error) {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()
	ledgers := make([]*SSLedger, 0)

	s.ledgers.Range(func(key, val interface{}) bool {
		if strings.HasPrefix(key.(string), s.prefix(s.currCycle)) {
			l := new(SSLedger)
			if e := copyLedger(val.(*SSLedger), l); e == nil {
				ledgers = append(ledgers, l)
			}
		}
		return true
	})
	return ledgers, nil
}

func (s *ssLedgerStore) Update(newCycle int64) error {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()
	cycles, loaded := s.ledgers.Load("pending")
	if loaded {
		cycles = append(cycles.([]int64), s.currCycle)
	} else {
		cycles = []int64{s.currCycle}
	}
	s.ledgers.Store("pending", cycles)
	s.currCycle = newCycle
	return nil
}

func (s *ssLedgerStore) BillingCycle() int64 {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()
	return s.currCycle
}

func (s *ssLedgerStore) Close() error {
	return nil
}

func (s *ssLedgerStore) GetPending() (map[int][]*SSLedger, error) {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()

	retVals := map[int][]*SSLedger{}
	pending, loaded := s.ledgers.Load("pending")
	if !loaded {
		return retVals, nil
	}
	s.ledgers.Range(func(key, val interface{}) bool {
		for _, v := range pending.([]int64) {
			if strings.HasPrefix(key.(string), s.prefix(v)) {
				rv, ok := retVals[int(v)]
				if !ok {
					rv = make([]*SSLedger, 0)
				}
				l := new(SSLedger)
				if e := copyLedger(val.(*SSLedger), l); e == nil {
					rv = append(rv, l)
					retVals[int(v)] = rv
				}
			}
		}
		return true
	})
	return retVals, nil
}

func (s *ssLedgerStore) ClearPending(cycles []int) ([]int, error) {
	s.cycleMtx.Lock()
	defer s.cycleMtx.Unlock()

	keysToDelete := []string{}
	s.ledgers.Range(func(key, val interface{}) bool {
		for _, v := range cycles {
			if strings.HasPrefix(key.(string), s.prefix(int64(v))) {
				keysToDelete = append(keysToDelete, key.(string))
			}
		}
		return true
	})
	cleared := []int{}
	pending, loaded := s.ledgers.Load("pending")
	if loaded {
		for _, c := range cycles {
			for idx, p := range pending.([]int64) {
				if int64(c) == p {
					pending = append(pending.([]int64)[:idx], pending.([]int64)[idx:]...)
					cleared = append(cleared, c)
					break
				}
			}
		}
	}
	for _, k := range keysToDelete {
		s.ledgers.Delete(k)
	}
	return cleared, nil
}
