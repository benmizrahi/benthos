package processor

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/benthosdev/benthos/v4/internal/batch"
	"github.com/benthosdev/benthos/v4/internal/bloblang/field"
	"github.com/benthosdev/benthos/v4/internal/component/metrics"
	"github.com/benthosdev/benthos/v4/internal/component/processor"
	"github.com/benthosdev/benthos/v4/internal/docs"
	"github.com/benthosdev/benthos/v4/internal/interop"
	"github.com/benthosdev/benthos/v4/internal/log"
	"github.com/benthosdev/benthos/v4/internal/message"
	"github.com/benthosdev/benthos/v4/internal/tracing"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeArchive] = TypeSpec{
		constructor: func(conf Config, mgr interop.Manager, log log.Modular, stats metrics.Type) (processor.V1, error) {
			p, err := newArchive(conf.Archive, mgr)
			if err != nil {
				return nil, err
			}
			return processor.NewV2BatchedToV1Processor("archive", p, stats), nil
		},
		Summary: `
Archives all the messages of a batch into a single message according to the
selected archive [format](#formats).`,
		Description: `
Some archive formats (such as tar, zip) treat each archive item (message part)
as a file with a path. Since message parts only contain raw data a unique path
must be generated for each part. This can be done by using function
interpolations on the 'path' field as described
[here](/docs/configuration/interpolation#bloblang-queries). For types that aren't file based
(such as binary) the file field is ignored.

The resulting archived message adopts the metadata of the _first_ message part
of the batch.`,
		Categories: []string{
			"Parsing", "Utility",
		},
		UsesBatches: true,
		Config: docs.FieldComponent().WithChildren(
			docs.FieldString("format", "The archiving [format](#formats) to apply.").HasOptions("tar", "zip", "binary", "lines", "json_array", "concatenate"),
			docs.FieldString(
				"path", "The path to set for each message in the archive (when applicable).",
				"${!count(\"files\")}-${!timestamp_unix_nano()}.txt", "${!meta(\"kafka_key\")}-${!json(\"id\")}.json",
			).IsInterpolated(),
		),
		Footnotes: `
## Formats

### ` + "`concatenate`" + `

Join the raw contents of each message into a single binary message.

### ` + "`tar`" + `

Archive messages to a unix standard tape archive.

### ` + "`zip`" + `

Archive messages to a zip file.

### ` + "`binary`" + `

Archive messages to a binary blob format consisting of:

- Four bytes containing number of messages in the batch (in big endian)
- For each message part:
  + Four bytes containing the length of the message (in big endian)
  + The content of message

### ` + "`lines`" + `

Join the raw contents of each message and insert a line break between each one.

### ` + "`json_array`" + `

Attempt to parse each message as a JSON document and append the result to an
array, which becomes the contents of the resulting message.

## Examples

If we had JSON messages in a batch each of the form:

` + "```json" + `
{"doc":{"id":"foo","body":"hello world 1"}}
` + "```" + `

And we wished to tar archive them, setting their filenames to their respective
unique IDs (with the extension ` + "`.json`" + `), our config might look like
this:

` + "```yaml" + `
pipeline:
  processors:
    - archive:
        format: tar
        path: ${!json("doc.id")}.json
` + "```" + ``,
	}
}

//------------------------------------------------------------------------------

// ArchiveConfig contains configuration fields for the Archive processor.
type ArchiveConfig struct {
	Format string `json:"format" yaml:"format"`
	Path   string `json:"path" yaml:"path"`
}

// NewArchiveConfig returns a ArchiveConfig with default values.
func NewArchiveConfig() ArchiveConfig {
	return ArchiveConfig{
		Format: "",
		Path:   ``,
	}
}

//------------------------------------------------------------------------------

type archiveFunc func(hFunc headerFunc, msg *message.Batch) (*message.Part, error)

type headerFunc func(index int, body *message.Part) os.FileInfo

func tarArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	// Iterate through the parts of the message.
	err := msg.Iter(func(i int, part *message.Part) error {
		hdr, err := tar.FileInfoHeader(hFunc(i, part), "")
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(part.Get()); err != nil {
			return err
		}
		return nil
	})
	tw.Close()

	if err != nil {
		return nil, err
	}
	newPart := msg.Get(0).Copy()
	newPart.Set(buf.Bytes())
	return newPart, nil
}

func zipArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// Iterate through the parts of the message.
	err := msg.Iter(func(i int, part *message.Part) error {
		h, err := zip.FileInfoHeader(hFunc(i, part))
		if err != nil {
			return err
		}
		h.Method = zip.Deflate

		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err = w.Write(part.Get()); err != nil {
			return err
		}
		return nil
	})
	zw.Close()

	if err != nil {
		return nil, err
	}
	newPart := msg.Get(0).Copy()
	newPart.Set(buf.Bytes())
	return newPart, nil
}

func binaryArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	newPart := msg.Get(0).Copy()
	newPart.Set(message.ToBytes(msg))
	return newPart, nil
}

func linesArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	tmpParts := make([][]byte, msg.Len())
	_ = msg.Iter(func(i int, part *message.Part) error {
		tmpParts[i] = part.Get()
		return nil
	})
	newPart := msg.Get(0).Copy()
	newPart.Set(bytes.Join(tmpParts, []byte("\n")))
	return newPart, nil
}

func concatenateArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	var buf bytes.Buffer
	_ = msg.Iter(func(i int, part *message.Part) error {
		buf.Write(part.Get())
		return nil
	})
	newPart := msg.Get(0).Copy()
	newPart.Set(buf.Bytes())
	return newPart, nil
}

func jsonArrayArchive(hFunc headerFunc, msg *message.Batch) (*message.Part, error) {
	var array []interface{}

	// Iterate through the parts of the message.
	err := msg.Iter(func(i int, part *message.Part) error {
		doc, jerr := part.JSON()
		if jerr != nil {
			return fmt.Errorf("failed to parse message as JSON: %v", jerr)
		}
		array = append(array, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}

	newPart := msg.Get(0).Copy()
	newPart.SetJSON(array)
	return newPart, nil
}

func strToArchiver(str string) (archiveFunc, error) {
	switch str {
	case "tar":
		return tarArchive, nil
	case "zip":
		return zipArchive, nil
	case "binary":
		return binaryArchive, nil
	case "lines":
		return linesArchive, nil
	case "json_array":
		return jsonArrayArchive, nil
	case "concatenate":
		return concatenateArchive, nil
	}
	return nil, fmt.Errorf("archive format not recognised: %v", str)
}

//------------------------------------------------------------------------------

type archive struct {
	archive archiveFunc
	path    *field.Expression
	log     log.Modular
}

func newArchive(conf ArchiveConfig, mgr interop.Manager) (processor.V2Batched, error) {
	path, err := mgr.BloblEnvironment().NewField(conf.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse path expression: %v", err)
	}
	archiver, err := strToArchiver(conf.Format)
	if err != nil {
		return nil, err
	}

	return &archive{
		archive: archiver,
		path:    path,
		log:     mgr.Logger(),
	}, nil
}

//------------------------------------------------------------------------------

type fakeInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f fakeInfo) Name() string {
	return f.name
}
func (f fakeInfo) Size() int64 {
	return f.size
}
func (f fakeInfo) Mode() os.FileMode {
	return f.mode
}
func (f fakeInfo) ModTime() time.Time {
	return time.Now()
}
func (f fakeInfo) IsDir() bool {
	return false
}
func (f fakeInfo) Sys() interface{} {
	return nil
}

func (d *archive) createHeaderFunc(msg *message.Batch) func(int, *message.Part) os.FileInfo {
	return func(index int, body *message.Part) os.FileInfo {
		return fakeInfo{
			name: d.path.String(index, msg),
			size: int64(len(body.Get())),
			mode: 0o666,
		}
	}
}

//------------------------------------------------------------------------------

func (d *archive) ProcessBatch(ctx context.Context, _ []*tracing.Span, msg *message.Batch) ([]*message.Batch, error) {
	if msg.Len() == 0 {
		return nil, nil
	}

	newMsg := msg.Copy()

	newPart, err := d.archive(d.createHeaderFunc(msg), msg)
	if err != nil {
		d.log.Errorf("Failed to create archive: %v\n", err)
		return nil, err
	}
	newPart = batch.WithCollapsedCount(newPart, msg.Len())
	newMsg.SetAll([]*message.Part{newPart})

	msgs := [1]*message.Batch{newMsg}
	return msgs[:], nil
}

func (d *archive) Close(context.Context) error {
	return nil
}
