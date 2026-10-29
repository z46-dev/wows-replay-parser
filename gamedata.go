package replayparser

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"

	wowsextractor "github.com/z46-dev/wows-extractor"
)

// GameData shares build assets and decoded catalogs from a caller-owned archive.
type GameData struct {
	archive *wowsextractor.Archive
	mu      sync.Mutex
	params  map[string]*wowsextractor.GameParams
}

// NewGameData borrows an opened extractor archive without taking ownership of it.
func NewGameData(archive *wowsextractor.Archive) (data *GameData, err error) {
	if archive == nil || archive.Manifest.Build == "" {
		err = fmt.Errorf("an opened extractor archive with a build is required")
		return
	}

	data = &GameData{
		archive: archive,
		params:  make(map[string]*wowsextractor.GameParams),
	}

	return
}

// Version returns the archive's declared client version, or its build number.
func (data *GameData) Version() (version string) {
	if data == nil {
		return
	}

	if version = data.archive.Manifest.ClientVersion; version == "" {
		version = data.archive.Manifest.Build
	}

	return
}

// ValidateVersion rejects a replay recorded with a different game build.
func (data *GameData) ValidateVersion(replay *Replay) (err error) {
	if data == nil || replay == nil {
		err = fmt.Errorf("replay and game data are required")
		return
	}

	var version string = strings.ReplaceAll(replay.Metadata.ClientVersionFromExe, ",", ".")
	if data.archive.Manifest.ClientVersion != "" {
		if data.archive.Manifest.ClientVersion != version {
			err = fmt.Errorf("archive version %s does not match replay version %s", data.archive.Manifest.ClientVersion, version)
		}
	} else if !strings.HasSuffix(version, "."+data.archive.Manifest.Build) {
		err = fmt.Errorf("archive build %s does not match replay version %s", data.archive.Manifest.Build, version)
	}

	return
}

// ReadFile reads a virtual game asset from the borrowed archive.
func (data *GameData) ReadFile(name string) (content []byte, err error) {
	if data == nil {
		err = fmt.Errorf("game data is required")
		return
	}

	content, err = fs.ReadFile(data.archive, name)
	return
}

// GameParams decodes each requested region once for reuse across replays.
func (data *GameData) GameParams(region string) (params *wowsextractor.GameParams, err error) {
	if data == nil || region == "" {
		err = fmt.Errorf("game data and region are required")
		return
	}

	data.mu.Lock()
	defer data.mu.Unlock()
	if params = data.params[region]; params == nil {
		if params, err = data.archive.LoadGameParams(region); err != nil {
			return
		}

		data.params[region] = params
	}

	return
}
