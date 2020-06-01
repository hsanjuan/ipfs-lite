package ss_ledger

import (
	"testing"
	"time"
)

func TestGetStore(t *testing.T) {

	st := NewMapLedgerStore()

	val := &SSLedger{
		Partner:            "partner1",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    1,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          100,
		BytesPayRecvd:      100,
		Sent:               0.1,
		Recvd:              0.1,
	}

	err := st.Store(val)
	if err != nil {
		t.Fatalf("Failed storing val %v Err:%s", val.Loggable(), err.Error())
	}

	val2 := &SSLedger{
		Partner: "partner1",
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

	val3 := &SSLedger{
		Partner: "partner2",
	}

	err = st.Get(val3)
	if err == nil {
		t.Fatalf("Store should have returned error for key")
	}

	val4 := &SSLedger{
		Partner:            "partner2",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    10,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          10000,
		BytesPayRecvd:      10000,
		Sent:               10,
		Recvd:              10,
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

	st := NewMapLedgerStore()

	l, err := st.List()
	if err != nil || len(l) != 0 {
		t.Fatal("Failed listing empty store")
	}

	val := &SSLedger{
		Partner:            "partner1",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    1,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          100,
		BytesPayRecvd:      100,
		Sent:               0.1,
		Recvd:              0.1,
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

	val2 := &SSLedger{
		Partner:            "partner2",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    10,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          10000,
		BytesPayRecvd:      10000,
		Sent:               10,
		Recvd:              10,
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
	if l[0].String() != val.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", l[0].Loggable(), val.Loggable())
	}

	if l[1].String() != val2.String() {
		t.Fatalf("Stored value unequal Got:%v Expected:%v", l[1].Loggable(), val2.Loggable())
	}
}

func TestCyclePending(t *testing.T) {

	st := NewMapLedgerStore()

	val := &SSLedger{
		Partner:            "partner1",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    1,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          100,
		BytesPayRecvd:      100,
		Sent:               0.1,
		Recvd:              0.1,
	}

	val2 := &SSLedger{
		Partner:            "partner2",
		LastMpExchange:     time.Now().Unix(),
		MpExchangeCount:    10,
		Whitelisted:        true,
		LastWhitelistCheck: time.Now().Unix(),
		BytesPaid:          10000,
		BytesPayRecvd:      10000,
		Sent:               10,
		Recvd:              10,
	}

	for i := 0; i < 3; i++ {
		if st.BillingCycle() != int64(i+1) {
			t.Fatalf("Invalid billing cycle")
		}

		err := st.Store(val)
		if err != nil {
			t.Fatalf("Failed storing val %v Err:%s", val.Loggable(), err.Error())
		}

		err = st.Store(val2)
		if err != nil {
			t.Fatalf("Failed storing val %v Err:%s", val2.Loggable(), err.Error())
		}

		err = st.Update(int64(i + 2))
		if err != nil {
			t.Fatalf("Failed updating cycle Err:%s", err.Error())
		}
	}
	if st.BillingCycle() != 4 {
		t.Fatalf("Invalid billing cycle")
	}
	l, err := st.List()
	if err != nil {
		t.Fatalf("Failed listing Err:%s", err.Error())
	}
	if len(l) != 0 {
		t.Fatal("Length should be 0. Latest cycle ended.")
	}
	pd, err := st.GetPending()
	if err != nil {
		t.Fatalf("Failed getting pending Err:%s", err.Error())
	}
	clearList := []int{}
	for k, v := range pd {
		if v[1].String() != val.String() {
			t.Fatalf("Stored value unequal Got:%v Expected:%v", v[1].Loggable(), val.Loggable())
		}
		if v[0].String() != val2.String() {
			t.Fatalf("Stored value unequal Got:%v Expected:%v", v[0].Loggable(), val.Loggable())
		}
		clearList = append(clearList, k)
	}
	cleared, err := st.ClearPending(clearList)
	if err != nil {
		t.Fatalf("Failed clearing pending Err:%s", err.Error())
	}
	if len(cleared) != len(clearList) {
		t.Fatalf("Did not clear all cycles")
	}
	newPd, err := st.GetPending()
	if err != nil {
		t.Fatalf("Failed getting pending Err:%s", err.Error())
	}
	if len(newPd) != 0 {
		t.Fatalf("All pending not cleared")
	}
}
