package generic

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/benthosdev/benthos/v4/internal/component"
	"github.com/benthosdev/benthos/v4/internal/component/output"
	"github.com/benthosdev/benthos/v4/internal/message"
	"github.com/benthosdev/benthos/v4/internal/shutdown"
)

type fanOutOutputBroker struct {
	transactions <-chan message.Transaction

	outputTSChans []chan message.Transaction
	outputs       []output.Streamed

	shutSig *shutdown.Signaller
}

func newFanOutOutputBroker(outputs []output.Streamed) (*fanOutOutputBroker, error) {
	o := &fanOutOutputBroker{
		transactions: nil,
		outputs:      outputs,
		shutSig:      shutdown.NewSignaller(),
	}

	o.outputTSChans = make([]chan message.Transaction, len(o.outputs))
	for i := range o.outputTSChans {
		o.outputTSChans[i] = make(chan message.Transaction)
		if err := o.outputs[i].Consume(o.outputTSChans[i]); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (o *fanOutOutputBroker) Consume(transactions <-chan message.Transaction) error {
	if o.transactions != nil {
		return component.ErrAlreadyStarted
	}
	o.transactions = transactions

	go o.loop()
	return nil
}

func (o *fanOutOutputBroker) Connected() bool {
	for _, out := range o.outputs {
		if !out.Connected() {
			return false
		}
	}
	return true
}

func (o *fanOutOutputBroker) loop() {
	ackInterruptChan := make(chan struct{})
	var ackPending int64

	defer func() {
		// Wait for pending acks to be resolved, or forceful termination
	ackWaitLoop:
		for atomic.LoadInt64(&ackPending) > 0 {
			select {
			case <-ackInterruptChan:
			case <-time.After(time.Millisecond * 100):
				// Just incase an interrupt doesn't arrive.
			case <-o.shutSig.CloseAtLeisureChan():
				break ackWaitLoop
			}
		}
		for _, c := range o.outputTSChans {
			close(c)
		}
		closeAllOutputs(o.outputs)
		o.shutSig.ShutdownComplete()
	}()

	for {
		var ts message.Transaction
		var open bool
		select {
		case ts, open = <-o.transactions:
			if !open {
				return
			}
		case <-o.shutSig.CloseAtLeisureChan():
			return
		}

		_ = atomic.AddInt64(&ackPending, 1)
		pendingResponses := int64(len(o.outputTSChans))
		for target := range o.outputTSChans {
			msgCopy, i := ts.Payload.Copy(), target
			select {
			case o.outputTSChans[i] <- message.NewTransactionFunc(msgCopy, func(ctx context.Context, err error) error {
				if atomic.AddInt64(&pendingResponses, -1) == 0 || err != nil {
					atomic.StoreInt64(&pendingResponses, 0)
					ackErr := ts.Ack(ctx, err)
					_ = atomic.AddInt64(&ackPending, -1)
					select {
					case ackInterruptChan <- struct{}{}:
					default:
					}
					return ackErr
				}
				return nil
			}):
			case <-o.shutSig.CloseAtLeisureChan():
				return
			}
		}
	}
}

func (o *fanOutOutputBroker) CloseAsync() {
	o.shutSig.CloseAtLeisure()
}

func (o *fanOutOutputBroker) WaitForClose(timeout time.Duration) error {
	select {
	case <-o.shutSig.HasClosedChan():
	case <-time.After(timeout):
		return component.ErrTimeout
	}
	return nil
}
