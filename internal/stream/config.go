package stream

import (
	"gopkg.in/yaml.v3"

	"github.com/benthosdev/benthos/v4/internal/component/buffer"
	"github.com/benthosdev/benthos/v4/internal/docs"
	"github.com/benthosdev/benthos/v4/internal/old/input"
	"github.com/benthosdev/benthos/v4/internal/old/output"
	"github.com/benthosdev/benthos/v4/internal/pipeline"
)

//------------------------------------------------------------------------------

// Config is a configuration struct representing all four layers of a Benthos
// stream.
type Config struct {
	Input    input.Config    `json:"input" yaml:"input"`
	Buffer   buffer.Config   `json:"buffer" yaml:"buffer"`
	Pipeline pipeline.Config `json:"pipeline" yaml:"pipeline"`
	Output   output.Config   `json:"output" yaml:"output"`
}

// NewConfig returns a new configuration with default values.
func NewConfig() Config {
	return Config{
		Input:    input.NewConfig(),
		Buffer:   buffer.NewConfig(),
		Pipeline: pipeline.NewConfig(),
		Output:   output.NewConfig(),
	}
}

// Sanitised returns a sanitised copy of the Benthos configuration, meaning
// fields of no consequence (unused inputs, outputs, processors etc) are
// excluded.
func (c Config) Sanitised() (interface{}, error) {
	var node yaml.Node
	if err := node.Encode(c); err != nil {
		return nil, err
	}

	if err := Spec().SanitiseYAML(&node, docs.SanitiseConfig{
		RemoveTypeField: true,
	}); err != nil {
		return nil, err
	}

	var g interface{}
	if err := node.Decode(&g); err != nil {
		return nil, err
	}
	return g, nil
}

//------------------------------------------------------------------------------
