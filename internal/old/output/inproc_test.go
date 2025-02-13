package output_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/benthosdev/benthos/v4/internal/component"
	"github.com/benthosdev/benthos/v4/internal/component/metrics"
	"github.com/benthosdev/benthos/v4/internal/log"
	"github.com/benthosdev/benthos/v4/internal/manager"
	"github.com/benthosdev/benthos/v4/internal/message"
	"github.com/benthosdev/benthos/v4/internal/old/output"

	_ "github.com/benthosdev/benthos/v4/public/components/all"
)

//------------------------------------------------------------------------------

func TestInproc(t *testing.T) {
	mgr, err := manager.NewV2(manager.NewResourceConfig(), nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Fatal(err)
	}

	if _, err = mgr.GetPipe("foo"); err != component.ErrPipeNotFound {
		t.Errorf("Wrong error returned: %v != %v", err, component.ErrPipeNotFound)
	}

	conf := output.NewConfig()
	conf.Inproc = "foo"

	ip, err := output.NewInproc(conf, mgr, log.Noop(), metrics.Noop())
	require.NoError(t, err)

	tinchan := make(chan message.Transaction)
	if err = ip.Consume(tinchan); err != nil {
		t.Fatal(err)
	}

	select {
	case tinchan <- message.NewTransaction(nil, nil):
	case <-time.After(time.Second):
		t.Error("Timed out")
	}

	var toutchan <-chan message.Transaction
	if toutchan, err = mgr.GetPipe("foo"); err != nil {
		t.Error(err)
	}

	select {
	case <-toutchan:
	case <-time.After(time.Second):
		t.Error("Timed out")
	}

	ip.CloseAsync()
	if err = ip.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}

	select {
	case _, open := <-toutchan:
		if open {
			t.Error("transaction chan not closed")
		}
	case <-time.After(time.Second):
		t.Error("Timed out")
	}
	if _, err = mgr.GetPipe("foo"); err != component.ErrPipeNotFound {
		t.Errorf("Wrong error returned: %v != %v", err, component.ErrPipeNotFound)
	}
}

//------------------------------------------------------------------------------
