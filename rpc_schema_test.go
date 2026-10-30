package replayparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	wowsextractor "github.com/z46-dev/wows-extractor"
)

// TestEntitySpecsUsesToolkitOrder checks alias, interface, method, and property ordering.
func TestEntitySpecsUsesToolkitOrder(t *testing.T) {
	var (
		path    string = filepath.Join(t.TempDir(), "build.zip")
		file    *os.File
		writer  *zip.Writer
		archive *wowsextractor.Archive
		data    *GameData
		specs   []EntitySpec
		err     error
	)

	if file, err = os.Create(path); !assert.NoError(t, err) {
		return
	}

	writer = zip.NewWriter(file)
	var entries map[string]string = map[string]string{
		".wows-extractor/manifest.json":             `{"formatVersion":2,"build":"13357625","clientVersion":"15.9.0.13357625"}`,
		"scripts/entity_defs/alias.xml":             `<root><SMALL>UINT8</SMALL></root>`,
		"scripts/entities.xml":                      `<root><ClientServerEntities><Avatar/></ClientServerEntities></root>`,
		"scripts/entity_defs/Avatar.def":            `<root><Implements><Shared>Shared</Shared></Implements><Properties><large><Type>UINT32</Type><Flags>OWN_CLIENT</Flags></large><small><Type>SMALL</Type><Flags>BASE_AND_CLIENT</Flags></small></Properties><ClientMethods><slow><Arg>UINT32</Arg></slow><fast><Arg>UINT8</Arg></fast></ClientMethods></root>`,
		"scripts/entity_defs/interfaces/Shared.def": `<root><Properties><inherited><Type>UINT16</Type><Flags>OWN_CLIENT</Flags></inherited></Properties><ClientMethods><shared><Arg>UINT16</Arg></shared></ClientMethods></root>`,
	}

	for name, content := range entries {
		var entry io.Writer
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

	if archive, err = wowsextractor.Load(path); !assert.NoError(t, err) {
		return
	}

	defer archive.Close()
	if data, err = NewGameData(archive); !assert.NoError(t, err) {
		return
	}

	if specs, err = data.EntitySpecs(); !assert.NoError(t, err) {
		return
	}

	if !assert.Len(t, specs, 1) {
		return
	}

	assert.Equal(t, []string{"small", "inherited", "large"}, []string{specs[0].Properties[0].Name, specs[0].Properties[1].Name, specs[0].Properties[2].Name})
	assert.Equal(t, []string{"fast", "shared", "slow"}, []string{specs[0].ClientMethods[0].Name, specs[0].ClientMethods[1].Name, specs[0].ClientMethods[2].Name})
	assert.Equal(t, "small", specs[0].BaseProperties[0].Name)
}
