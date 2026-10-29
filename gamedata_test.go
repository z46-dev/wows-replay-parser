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

// TestGameDataVersionGate checks a borrowed archive against its replay build.
func TestGameDataVersionGate(t *testing.T) {
	var (
		archive *wowsextractor.Archive = &wowsextractor.Archive{Manifest: wowsextractor.Manifest{Build: "13357625", ClientVersion: "15.9.0.13357625"}}
		data    *GameData
		err     error
	)

	if data, err = NewGameData(archive); !assert.NoError(t, err) {
		return
	}

	err = data.ValidateVersion(&Replay{Metadata: Metadata{ClientVersionFromExe: "15,9,0,13357625"}})
	assert.NoError(t, err)

	err = data.ValidateVersion(&Replay{Metadata: Metadata{ClientVersionFromExe: "15,8,0,13187581"}})
	assert.Error(t, err)
}

// TestGameDataBorrowsArchive checks that both users can read the same opened ZIP.
func TestGameDataBorrowsArchive(t *testing.T) {
	var (
		path    string = filepath.Join(t.TempDir(), "game.zip")
		file    *os.File
		writer  *zip.Writer
		entry   io.Writer
		archive *wowsextractor.Archive
		data    *GameData
		content []byte
		err     error
	)

	if file, err = os.Create(path); !assert.NoError(t, err) {
		return
	}

	defer file.Close()
	writer = zip.NewWriter(file)

	if entry, err = writer.Create(".wows-extractor/manifest.json"); !assert.NoError(t, err) {
		return
	}

	if _, err = entry.Write([]byte(`{"formatVersion":2,"build":"13357625","clientVersion":"15.9.0.13357625"}`)); !assert.NoError(t, err) {
		return
	}

	if entry, err = writer.Create("scripts/entities.xml"); !assert.NoError(t, err) {
		return
	}

	if _, err = entry.Write([]byte("<root/>")); !assert.NoError(t, err) {
		return
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

	content, err = data.ReadFile("scripts/entities.xml")
	assert.NoError(t, err)
	assert.Equal(t, "<root/>", string(content))

	content, err = archive.ReadFile("scripts/entities.xml")
	assert.NoError(t, err)
	assert.Equal(t, "<root/>", string(content))
}
