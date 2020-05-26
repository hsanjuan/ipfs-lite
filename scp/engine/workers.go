package engine

import (
	"context"
	"errors"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"time"
)

func (e *Engine) ssTaskWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5)
	handshakeChecker := time.NewTicker(time.Second * 30)
	checkAndUpdate := func() {
		// Check for billing cycle
		if e.getBillingCycle() != e.ssStore.BillingCycle() {
			// Update the billing cycle
			if e.getBillingCycle() < e.ssStore.BillingCycle() {
				// Ledger store should never be ahead of epoch
				panic("Billing cycle in SS_Store is greater!")
			}
			e.endBillingCycle(e.getBillingCycle())
		}
	}
	defer ticker.Stop()
	defer handshakeChecker.Stop()
	defer close(e.sentQueue)
	//call initially
	checkAndUpdate()
	for {
		select {
		case <-ticker.C:
			checkAndUpdate()
		case <-handshakeChecker.C:
			outStanding := make([]peer.ID, 0)
			done := false
			for {
				select {
				case p := <-e.sentQueue:
					if !e.handshakeDone(p) {
						outStanding = append(outStanding, p)
						log.Infof("Handshake not yet done for %s. Resend msg.", p.Pretty())
					}
				case <-time.After(time.Second):
					log.Infof("Waited 1 second for outstanding handshake queue")
					done = true
				}
				if done {
					break
				}
			}
			log.Infof("Outstanding handshakes %d", len(outStanding))
			for _, v := range outStanding {
				// This will check again and send the message this time. Also
				// it will add the peer back to sentQueue
				_ = e.HandshakeDone(v)
			}
		case <-ctx.Done():
			// Program exiting
			return
		}
	}
}

func (e *Engine) taskWorker(ctx context.Context) {
	defer e.taskWorkerExit()
	for {
		oneTimeUse := make(chan *Envelope, 1) // buffer to prevent blocking
		select {
		case <-ctx.Done():
			return
		case e.outbox <- oneTimeUse:
		}
		// receiver is ready for an outoing envelope. let's prepare one. first,
		// we must acquire a task from the PQ...
		envelope, err := e.nextEnvelope(ctx)
		if err != nil {
			close(oneTimeUse)
			return // ctx cancelled
		}
		oneTimeUse <- envelope // buffered. won't block
		close(oneTimeUse)
	}
}

// taskWorkerExit handles cleanup of task workers
func (e *Engine) taskWorkerExit() {
	e.taskWorkerLock.Lock()
	defer e.taskWorkerLock.Unlock()

	e.taskWorkerCount--
	if e.taskWorkerCount == 0 {
		close(e.outbox)
	}
}

// nextEnvelope runs in the taskWorker goroutine. Returns an error if the
// context is cancelled before the next Envelope can be created.
func (e *Engine) nextEnvelope(ctx context.Context) (*Envelope, error) {
	for {
		p, nextTasks, _ := e.peerRequestQueue.PopTasks(1)
		for len(nextTasks) == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-e.workSignal:
				p, nextTasks, _ = e.peerRequestQueue.PopTasks(1)
			case <-e.ticker.C:
				// When a task is cancelled, the queue may be "frozen" for a
				// period of time. We periodically "thaw" the queue to make
				// sure it doesn't get stuck in a frozen state.
				e.peerRequestQueue.ThawRound()
				p, nextTasks, _ = e.peerRequestQueue.PopTasks(1)
			}
		}

		ct := nextTasks[0]
		currEnv, ok := ct.Data.(*Envelope)
		if !ok {
			log.Errorf("Invalid object type in task %v", ct.Data)
			return nil, errors.New("SCP Msg: Invalid object type")
		}

		log.Debugf("SCP engine -> msg %s to %s", ct.Topic.(string), p)
		return &Envelope{
			Peer:    p,
			Message: currEnv.Message,
			Sent: func() {
				// Callback for msg sent
				currEnv.Sent()
				// Once the message has been sent, signal the request queue so
				// it can be cleared from the queue
				e.peerRequestQueue.TasksDone(p, ct)
				// Signal the worker to check for more work
				e.signalNewWork()
			},
		}, nil
	}
}
