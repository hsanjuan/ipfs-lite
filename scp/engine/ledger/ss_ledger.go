package ss_ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/boltdb/bolt"
	pb "github.com/golang/protobuf/proto"
	logging "github.com/ipfs/go-log"
)

//go:generate protoc --proto_path=. --go_out=. ss_ledger.proto

var log = logging.Logger("ss_ledger")

const (
	BUCKET string = "Main"
)

var storeName = func(cycle int) string {
	return ".ssLedgerStore_" + strconv.Itoa(cycle) + ".DB"
}

var storePath = func(root string, cycle int) string {
	return fmt.Sprintf(storeFmt(root), cycle)
}

var pendingPath = func(root string, cycle int) string {
	return fmt.Sprintf(pendingFmt(root), cycle)
}

var storeFmt = func(root string) string {
	return root + string(os.PathSeparator) + ".ssLedgerStore_%d.DB"
}

var pendingFmt = func(root string) string {
	return root + string(os.PathSeparator) + ".ssLedgerStore_%d.DB.pending"
}

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
	rootPath  string
	currCycle int64
	dbP       *bolt.DB
	lk        sync.Mutex
}

func NewStore(rootPath string) (Store, error) {
	store := new(ssLedgerStore)

	if _, e := os.Stat(rootPath); e != nil {
		return nil, e
	}

	// Currently we will do a best effort to figure out the billing cycle
	// if there are no cycles present, we will initialize with cycle no 1
	currCycle := 1
	_ = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if path == rootPath {
			return filepath.SkipDir
		}
		var cycle int
		if _, e := fmt.Sscanf(path, storeFmt(rootPath), &cycle); e == nil {
			if cycle > currCycle {
				currCycle = cycle
			}
		} else {
			log.Errorf("Path not a DB %s Root: %s Error:%s", path, rootPath, e.Error())
		}
		return nil
	})
	log.Debugf("Found current cycle %d", currCycle)

	store.rootPath = rootPath
	store.currCycle = int64(currCycle)
	e := store.createDB()
	if e != nil {
		return nil, e
	}
	return store, nil
}

func newPendingStore(rootPath string, cycle int64) (Store, error) {
	fullName := pendingPath(rootPath, int(cycle))
	db, e := bolt.Open(fullName, 0600, nil)
	if e != nil {
		return nil, e
	}
	return &ssLedgerStore{rootPath: rootPath, currCycle: cycle, dbP: db}, nil
}

func (s *ssLedgerStore) createDB() error {
	s.lk.Lock()
	defer s.lk.Unlock()

	fullName := storePath(s.rootPath, int(s.currCycle))
	db, e := bolt.Open(fullName, 0600, nil)
	if e != nil {
		return e
	}

	if s.dbP != nil {
		s.dbP.Close()
	}

	s.dbP = db
	return nil
}

func (s *ssLedgerStore) Get(val *SSLedger) error {
	s.lk.Lock()

	err := s.dbP.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(BUCKET))
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}

		buf := bkt.Get([]byte(val.Partner))
		if buf != nil {
			err := pb.Unmarshal(buf, val)
			return err
		}

		return bolt.ErrKeyRequired
	})
	s.lk.Unlock()
	if err == bolt.ErrBucketNotFound {
		err = s.Store(val)
	}
	return err
}

func (s *ssLedgerStore) Store(val *SSLedger) error {
	s.lk.Lock()
	defer s.lk.Unlock()

	return s.dbP.Update(func(tx *bolt.Tx) error {

		bkt, err := tx.CreateBucketIfNotExists([]byte(BUCKET))
		if err != nil {
			return err
		}

		buf, err := pb.Marshal(val)
		if err != nil {
			return err
		}

		err = bkt.Put([]byte(val.Partner), buf)
		return err
	})
}

func (s *ssLedgerStore) List() ([]*SSLedger, error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	ledgers := make([]*SSLedger, 0)

	err := s.dbP.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(BUCKET))
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}

		c := bkt.Cursor()

		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			l := new(SSLedger)
			e := pb.Unmarshal(v, l)
			if e != nil {
				return e
			}
			log.Debugf("Read ledger %v", l.String())
			ledgers = append(ledgers, l)
		}
		return nil
	})
	return ledgers, err
}

func (s *ssLedgerStore) Update(newCycle int64) error {
	s.lk.Lock()
	// Close existing DB if daemon is running
	if s.dbP != nil {
		s.dbP.Close()
	}
	// Move current DB to pending
	err := os.Rename(storePath(s.rootPath, int(s.currCycle)),
		pendingPath(s.rootPath, int(s.currCycle)))
	if err != nil {
		log.Errorf("Failed renaming old ledger %s Err:%s", storePath, err.Error())
		// Ideally if we are not able to update the old ledger to pending
		// we shouldnt end the billing cycle. See how to handle this better.
		s.lk.Unlock()
		return err
	}
	s.currCycle = newCycle
	s.lk.Unlock()
	return s.createDB()
}

func (s *ssLedgerStore) BillingCycle() int64 {
	s.lk.Lock()
	defer s.lk.Unlock()
	return s.currCycle
}

func (s *ssLedgerStore) Close() error {
	s.lk.Lock()
	defer s.lk.Unlock()
	return s.dbP.Close()
}

func (s *ssLedgerStore) GetPending() (map[int][]*SSLedger, error) {
	pendingStores := make([]string, 0)
	err := filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ".pending") {
			pendingStores = append(pendingStores, path)
		}
		return nil
	})
	if err != nil && len(pendingStores) == 0 {
		log.Errorf("Failed listing any pending store files. Err:%s", err.Error())
		return map[int][]*SSLedger{}, err
	}
	log.Debugf("Pending Stores %v", pendingStores)
	if len(pendingStores) > 0 {
		res := make(map[int][]*SSLedger)
		for _, v := range pendingStores {
			var billingCycle int
			n, err := fmt.Sscanf(v, pendingFmt(s.rootPath), &billingCycle)
			if err != nil || n != 1 {
				log.Errorf("Failed parsing pending store name %s Err:%v n:%d", v, err, n)
				continue
			}
			st, err := newPendingStore(s.rootPath, int64(billingCycle))
			if err != nil {
				log.Errorf("Failed creating pending store object Err:%s", err.Error())
				continue
			}
			list, err := st.List()
			// This is best effort. If there is an issue on listing we
			// just dont add it. This happens for the 1st cycle.
			if err == nil {
				res[billingCycle] = list
			}
			st.Close()
		}
		return res, nil
	}
	return map[int][]*SSLedger{}, nil
}

func (s *ssLedgerStore) ClearPending(cycles []int) ([]int, error) {
	pendingStores := make([]string, 0)
	if len(cycles) == 0 {
		err := filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
			if strings.Contains(path, ".pending") {
				pendingStores = append(pendingStores, path)
			}
			return nil
		})
		if err != nil {
			log.Errorf("Failed listing pending store files. Err:%s", err.Error())
			return []int{}, err
		}
	} else {
		for _, v := range cycles {
			pendingStores = append(pendingStores, pendingPath(s.rootPath, v))
		}
	}
	log.Debugf("List of pending stores to clear %v", pendingStores)
	res := make([]int, len(pendingStores))
	for i, v := range pendingStores {
		_ = os.Remove(v)
		_, _ = fmt.Sscanf(v, pendingFmt(s.rootPath), &res[i])
	}
	log.Debugf("Result of ClearPending %v", res)
	return res, nil
}
