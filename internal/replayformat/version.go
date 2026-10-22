// Package replayformat catalogs observed client builds and replay container metadata.
// It does not establish compatibility between packet payload decoders.
package replayformat

import "strings"

// Magic identifies a World of Warships replay file.
const Magic uint32 = 0x11343212

// ClientBuild describes an observed client build and its replay block count.
type ClientBuild struct {
	ClientVersion string
	BlockCount    uint32
}

var clientBuilds = map[string]ClientBuild{
	"15.8.0.13187581": {
		ClientVersion: "15.8.0.13187581",
		BlockCount:    3,
	},
	"15.9.0.13357625": {
		ClientVersion: "15.9.0.13357625",
		BlockCount:    3,
	},
}

// LookupClientBuild returns observed metadata for a replay header's client version.
func LookupClientBuild(clientVersion string) (build ClientBuild, ok bool) {
	build, ok = clientBuilds[strings.ReplaceAll(clientVersion, ",", ".")]

	return
}
