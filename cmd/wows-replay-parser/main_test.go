package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
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

// TestEventsCommandStreamsFilteredJSON checks the CLI shares the library event walk.
func TestEventsCommandStreamsFilteredJSON(t *testing.T) {
	var (
		path   string = filepath.Join(t.TempDir(), "build.zip")
		file   *os.File
		writer *zip.Writer
		entry  io.Writer
		output bytes.Buffer
		err    error
	)
	if file, err = os.Create(path); !assert.NoError(t, err) {
		return
	}
	writer = zip.NewWriter(file)
	for name, content := range map[string]string{
		".wows-extractor/manifest.json": `{"formatVersion":2,"build":"13357625","clientVersion":"15.9.0.13357625"}`,
		"scripts/entity_defs/alias.xml": `<root/>`,
		"scripts/entities.xml":          `<root><ClientServerEntities/></root>`,
	} {
		if entry, err = writer.Create(name); !assert.NoError(t, err) {
			return
		}
		if _, err = entry.Write([]byte(content)); !assert.NoError(t, err) {
			return
		}
	}
	if err = writer.Close(); !assert.NoError(t, err) {
		return
	}
	if err = file.Close(); !assert.NoError(t, err) {
		return
	}

	err = run(arguments{Events: &eventsCommand{Replay: filepath.Join("..", "..", "testdata", "replays", "15.9_20261011_201842.wowsreplay"), GameData: path, Type: "battle_results"}}, &output)
	if !assert.NoError(t, err) {
		return
	}
	var event map[string]any
	if err = json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "battle_results", event["type"])
	assert.Equal(t, 1, bytes.Count(output.Bytes(), []byte{'\n'}))
}
