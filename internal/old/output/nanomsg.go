package output

import (
	"github.com/benthosdev/benthos/v4/internal/component/metrics"
	"github.com/benthosdev/benthos/v4/internal/component/output"
	"github.com/benthosdev/benthos/v4/internal/docs"
	"github.com/benthosdev/benthos/v4/internal/interop"
	"github.com/benthosdev/benthos/v4/internal/log"
	"github.com/benthosdev/benthos/v4/internal/old/output/writer"
)

//------------------------------------------------------------------------------

func init() {
	Constructors["nanomsg"] = TypeSpec{
		constructor: fromSimpleConstructor(NewNanomsg),
		Summary: `
Send messages over a Nanomsg socket.`,
		Description: `
Currently only PUSH and PUB sockets are supported.`,
		Async: true,
		Config: docs.FieldComponent().WithChildren(
			docs.FieldString("urls", "A list of URLs to connect to. If an item of the list contains commas it will be expanded into multiple URLs.", []string{"tcp://localhost:5556"}).Array(),
			docs.FieldBool("bind", "Whether the URLs listed should be bind (otherwise they are connected to)."),
			docs.FieldString("socket_type", "The socket type to send with.").HasOptions("PUSH", "PUB"),
			docs.FieldString("poll_timeout", "The maximum period of time to wait for a message to send before the request is abandoned and reattempted."),
			docs.FieldInt("max_in_flight", "The maximum number of messages to have in flight at a given time. Increase this to improve throughput."),
		),
		Categories: []string{
			"Network",
		},
	}
}

//------------------------------------------------------------------------------

// NewNanomsg creates a new Nanomsg output type.
func NewNanomsg(conf Config, mgr interop.Manager, log log.Modular, stats metrics.Type) (output.Streamed, error) {
	s, err := writer.NewNanomsg(conf.Nanomsg, log, stats)
	if err != nil {
		return nil, err
	}
	a, err := NewAsyncWriter(TypeNanomsg, conf.Nanomsg.MaxInFlight, s, log, stats)
	if err != nil {
		return nil, err
	}
	return OnlySinglePayloads(a), nil
}

//------------------------------------------------------------------------------
