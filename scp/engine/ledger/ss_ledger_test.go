package ss_ledger

import (
	"os"
	"testing"
	"time"
)

func TestGetStore(t *testing.T) {

	_ = os.Remove(".ssLedgerStore_1.DB")

	st, err := NewStore(".")
	if err != nil {
		t.Fatal("Failed opening store Err:" + err.Error())
	}

	val := &SSLedger{
		SSLedger: &SSLedger{
			Partner:            "partner1",
			LastMpExchange:     time.Now().Unix(),
			MpExchangeCount:    1,
			Whitelisted:        true,
			LastWhitelistCheck: time.Now().Unix(),
			BytesPaid:          100,
			BytesPayRecvd:      100,
			Sent:               0.1,
			Recvd:              0.1,
		},
	}

	err = st.Store(val)
	if err != nil {
		t.Fatalf("Failed storing val %v Err:%s", val.Loggable(), err.Error())
	}

	val2 := &ssLedger{
		SSLedger: &pb.SSLedger{
			Partner: "partner1",
		},
	}

	err = st.Get(val2)
	if err != nil {
		t.Fatalf("Failed getting val %v Err:%s", val.Loggable(), err.Error())
	}

	if val.String() != val2.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", val2.Loggable(), val.Loggable())
	}

	val.BytesPaid = 1000
	val.Sent = 1
	val.BytesPayRecvd = 1000
	val.Recvd = 1

	err = st.Store(val)
	if err != nil {
		t.Fatalf("Failed storing updated val %v Err:%s", val.Loggable(), err.Error())
	}

	err = st.Get(val2)
	if err != nil {
		t.Fatalf("Failed getting updated val %v Err:%s", val.Loggable(), err.Error())
	}

	if val.String() != val2.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", val2.Loggable(), val.Loggable())
	}

	val3 := &ssLedger{
		SSLedger: &pb.SSLedger{
			Partner: "partner2",
		},
	}

	err = st.Get(val3)
	if err == nil {
		t.Fatalf("Store should have returned error for key")
	}

	val4 := &ssLedger{
		SSLedger: &pb.SSLedger{
			Partner:            "partner2",
			LastMpExchange:     time.Now().Unix(),
			MpExchangeCount:    10,
			Whitelisted:        true,
			LastWhitelistCheck: time.Now().Unix(),
			BytesPaid:          10000,
			BytesPayRecvd:      10000,
			Sent:               10,
			Recvd:              10,
		},
	}

	err = st.Store(val4)
	if err != nil {
		t.Fatalf("Failed storing updated val %v Err:%s", val4.Loggable(), err.Error())
	}

	err = st.Get(val3)
	if err != nil {
		t.Fatalf("Failed getting updated val %v Err:%s", val4.Loggable(), err.Error())
	}

	if val4.String() != val3.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", val3.Loggable(), val4.Loggable())
	}
}

func TestStoreList(t *testing.T) {

	_ = os.Remove(".ssLedgerStore_1.DB")

	st, err := NewStore(".")
	if err != nil {
		t.Fatal("Failed opening store Err:" + err.Error())
	}

	l, err := st.List()
	if err == nil {
		t.Fatal("Succeeded listing empty store")
	}

	val := &ssLedger{
		SSLedger: &pb.SSLedger{
			Partner:            "partner1",
			LastMpExchange:     time.Now().Unix(),
			MpExchangeCount:    1,
			Whitelisted:        true,
			LastWhitelistCheck: time.Now().Unix(),
			BytesPaid:          100,
			BytesPayRecvd:      100,
			Sent:               0.1,
			Recvd:              0.1,
		},
	}

	err = st.Store(val)
	if err != nil {
		t.Fatalf("Failed storing val %v Err:%s", val.Loggable(), err.Error())
	}

	l, err = st.List()
	if err != nil {
		t.Fatalf("Failed listing Err:%s", err.Error())
	}

	if len(l) != 1 {
		t.Fatal("Length should be 1")
	}

	if l[0].String() != val.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", l[0].Loggable(), val.Loggable())
	}

	val2 := &ssLedger{
		SSLedger: &pb.SSLedger{
			Partner:            "partner2",
			LastMpExchange:     time.Now().Unix(),
			MpExchangeCount:    10,
			Whitelisted:        true,
			LastWhitelistCheck: time.Now().Unix(),
			BytesPaid:          10000,
			BytesPayRecvd:      10000,
			Sent:               10,
			Recvd:              10,
		},
	}

	err = st.Store(val2)
	if err != nil {
		t.Fatalf("Failed storing val %v Err:%s", val2.Loggable(), err.Error())
	}

	l, err = st.List()
	if err != nil {
		t.Fatalf("Failed listing Err:%s", err.Error())
	}

	if len(l) != 2 {
		t.Fatal("Length should be 2")
	}

	// List will return reverse order
	if l[1].String() != val.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", l[0].Loggable(), val.Loggable())
	}

	if l[0].String() != val2.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", l[1].Loggable(), val2.Loggable())
	}
}
