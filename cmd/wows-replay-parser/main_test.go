package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCommandsUseLibrary checks both CLI paths against a real replay fixture.
func TestCommandsUseLibrary(t *testing.T) {
	var (
		path   string = filepath.Join("..", "..", "testdata", "replays", "15.9_20261011_201842.wowsreplay")
		output bytes.Buffer
		err    error
	)

	if err = run(arguments{Inspect: &inspectCommand{Replay: path, JSON: true}}, &output); !assert.NoError(t, err) {
		return
	}
``
	var metadata map[string]any
	if err = json.Unmarshal(output.Bytes(), &metadata); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "15,9,0,13357625", metadata["clientVersionFromExe"])
	output.Reset()

	if err = run(arguments{Packets: &packetsCommand{Replay: path, JSON: true}}, &output); !assert.NoError(t, err) {
		return
	}

	var summary packetSummary
	if err = json.Unmarshal(output.Bytes(), &summary); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, 64160, summary.Total)
	assert.Equal(t, 1, summary.ByType["battle_results"])
}
