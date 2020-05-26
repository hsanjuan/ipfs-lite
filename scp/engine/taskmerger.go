package engine

import (
	mppb "github.com/StreamSpace/ss-light-client/scp/message/micropayment"
	"github.com/ipfs/go-peertaskqueue/peertask"
)

type taskMerger struct{}

func newTaskMerger() *taskMerger {
	return &taskMerger{}
}

// The request queue uses this Method to decide if a newly pushed task has any
// new information beyond the tasks with the same Topic (CID) in the queue.
func (*taskMerger) HasNewInfo(task peertask.Task, existing []peertask.Task) bool {
	if task.Topic.(string) == "Handshake" {
		return false
	}
	if task.Topic.(string) == "Micropayment" {
		ctd := (task.Data.(*Envelope)).Message.(*mppb.MicropaymentMsg)
		for _, et := range existing {
			etd := (et.Data.(*Envelope)).Message.(*mppb.MicropaymentMsg)
			if ctd.BillingCycle > etd.BillingCycle {
				return true
			}
			if ctd.Amount > etd.Amount {
				return true
			}
		}
	}
	return false
}

// The request queue uses Merge to merge a newly pushed task with an existing
// task with the same Topic (CID)
func (*taskMerger) Merge(task peertask.Task, existing *peertask.Task) {
	if task.Topic.(string) == "Handshake" {
		existing.Data = task.Data
		return
	}
	if task.Topic.(string) == "Micropayment" {
		ctd := (task.Data.(*Envelope)).Message.(*mppb.MicropaymentMsg)
		etd := (existing.Data.(*Envelope)).Message.(*mppb.MicropaymentMsg)
		if ctd.BillingCycle > etd.BillingCycle || ctd.Amount > etd.Amount {
			existing.Data = task.Data
		}
	}
}
