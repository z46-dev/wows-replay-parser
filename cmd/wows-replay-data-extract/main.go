package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
)

type (
	arguments struct {
		InstallPath string `arg:"--install,required" placeholder:"PATH" help:"World of Warships install directory"`
		Version     string `arg:"--version,required" placeholder:"VERSION" help:"Exact replay client version, for example 15.9.0.13357625"`
		OutputPath  string `arg:"--output" placeholder:"PATH" help:"Output directory (default references/out/VERSION)"`
		WowsUnpack  string `arg:"--wowsunpack" placeholder:"PATH" help:"wowsunpack executable (default: search PATH)"`
		Force       bool   `arg:"--force" help:"Replace an existing package for this version"`
	}

	manifest struct {
		FormatVersion int               `json:"formatVersion"`
		ClientVersion string            `json:"clientVersion"`
		Files         map[string]string `json:"files"`
	}
)

// main extracts the replay schemas from an unpacked World of Warships install.
func main() {
	var args arguments
	arg.MustParse(&args)

	if err := extract(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// extract copies only entity schemas required to interpret replay entity packets.
func extract(args arguments) (err error) {
	var (
		scriptsPath       string = filepath.Join(args.InstallPath, "res_unpack", "scripts")
		gameParamsPath    string = filepath.Join(args.InstallPath, "res_unpack", "content", "GameParams.data")
		silhouettesPath   string = filepath.Join(args.InstallPath, "res_unpack", "gui", "ships_silhouettes")
		classIconsPath    string = filepath.Join(args.InstallPath, "res_unpack", "gui", "fla", "minimap", "ship_icons")
		squadronIconsPath string = filepath.Join(args.InstallPath, "res_unpack", "gui", "grand_strategy_pass", "pawns")
		planeIconsPath    string = filepath.Join(args.InstallPath, "res_unpack", "gui", "battle_hud", "markers_minimap", "plane")
		buildNumber       string = strings.TrimPrefix(args.Version[strings.LastIndex(args.Version, ".")+1:], ".")
		localizationPath  string = filepath.Join(args.InstallPath, "bin", buildNumber, "res", "texts", "en", "LC_MESSAGES", "global.mo")
		outputPath        string = args.OutputPath
		wowsUnpackPath    string = args.WowsUnpack
		stagingPath       string
	)

	if outputPath == "" {
		outputPath = filepath.Join("references", "out", args.Version)
	}

	if !validVersion(args.Version) {
		err = fmt.Errorf("invalid version %q: use digits and dots", args.Version)
		return
	}

	if _, err = os.Stat(filepath.Join(scriptsPath, "entities.xml")); err != nil {
		err = fmt.Errorf("install does not contain res_unpack/scripts/entities.xml: %w", err)
		return
	}

	if _, err = os.Stat(filepath.Join(scriptsPath, "entity_defs", "alias.xml")); err != nil {
		err = fmt.Errorf("install does not contain unpacked entity definitions: %w", err)
		return
	}

	if _, err = os.Stat(gameParamsPath); err != nil {
		err = fmt.Errorf("install does not contain res_unpack/content/GameParams.data: %w", err)
		return
	}

	if _, err = os.Stat(silhouettesPath); err != nil {
		err = fmt.Errorf("install does not contain unpacked ship silhouettes: %w", err)
		return
	}

	if _, err = os.Stat(squadronIconsPath); err != nil {
		err = fmt.Errorf("install does not contain the unpacked aircraft silhouette: %w", err)
		return
	}

	if _, err = os.Stat(localizationPath); err != nil {
		err = fmt.Errorf("install does not contain English ship-name catalog for build %s: %w", buildNumber, err)
		return
	}

	if wowsUnpackPath == "" {
		if wowsUnpackPath, err = exec.LookPath("wowsunpack"); err != nil {
			err = fmt.Errorf("wowsunpack was not found on PATH; install it or pass --wowsunpack PATH")
			return
		}
	}

	if _, err = os.Stat(outputPath); err == nil && !args.Force {
		err = fmt.Errorf("output %q already exists; use --force to replace it", outputPath)
		return
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}

	if err = os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return
	}

	if stagingPath, err = os.MkdirTemp(filepath.Dir(outputPath), ".wows-replay-data-*"); err != nil {
		return
	}

	defer os.RemoveAll(stagingPath)

	var files map[string]string = make(map[string]string)
	if err = copyFile(filepath.Join(scriptsPath, "entities.xml"), filepath.Join(stagingPath, "entities.xml"), "entities.xml", files); err != nil {
		return
	}

	var definitionsPath string = filepath.Join(scriptsPath, "entity_defs")
	if err = filepath.WalkDir(definitionsPath, func(source string, entry os.DirEntry, walkErr error) (callbackErr error) {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return
		}

		var relativePath string
		if relativePath, callbackErr = filepath.Rel(definitionsPath, source); callbackErr != nil {
			return
		}

		if filepath.Ext(relativePath) != ".def" && filepath.Base(relativePath) != "alias.xml" {
			return
		}

		var packagePath string = filepath.Join("entity_defs", relativePath)
		callbackErr = copyFile(source, filepath.Join(stagingPath, packagePath), packagePath, files)
		return
	}); err != nil {
		return
	}

	var (
		gameParamsJSONPath           string    = filepath.Join(stagingPath, "GameParams.json")
		command                      *exec.Cmd = exec.Command(wowsUnpackPath, "game-params", "--input", gameParamsPath, "--id", "NA", "--ugly", gameParamsJSONPath) // #nosec G204 -- executable is user-selected; fixed args are passed without a shell.
		commandOutput                []byte
		paramsFile, localizationFile *os.File
		localizedNames               map[string]string
		vehicles                     []replaydata.Vehicle
		planeIcons                   []replaydata.PlaneIcon
	)

	if localizationFile, err = os.Open(localizationPath); err != nil { // #nosec G304 -- validated version path under the selected install.
		return
	}

	if localizedNames, err = replaydata.ParseMO(localizationFile); err != nil {
		localizationFile.Close() // #nosec G104 -- parse error is authoritative; close is cleanup only.
		err = fmt.Errorf("parse English ship-name catalog: %w", err)
		return
	}

	if err = localizationFile.Close(); err != nil {
		return
	}

	if commandOutput, err = command.CombinedOutput(); err != nil {
		err = fmt.Errorf("extract GameParams.data with wowsunpack: %w: %s", err, strings.TrimSpace(string(commandOutput)))
		return
	}

	if paramsFile, err = os.Open(gameParamsJSONPath); err != nil { // #nosec G304 -- generated path is inside the staging directory.
		return
	}

	defer paramsFile.Close()

	if vehicles, err = replaydata.ParseVehicles(paramsFile, localizedNames); err != nil {
		err = fmt.Errorf("parse GameParams ship catalog: %w", err)
		return
	}

	if err = paramsFile.Close(); err != nil {
		return
	}
	if paramsFile, err = os.Open(gameParamsJSONPath); err != nil { // #nosec G304 -- generated path is inside the staging directory.
		return
	}
	if planeIcons, err = replaydata.ParsePlaneIcons(paramsFile); err != nil {
		err = fmt.Errorf("parse GameParams aircraft icon catalog: %w", err)
		err = errors.Join(err, paramsFile.Close())
		return
	}
	if err = paramsFile.Close(); err != nil {
		return
	}

	if err = os.Remove(gameParamsJSONPath); err != nil {
		return
	}

	var vehicleData []byte
	if vehicleData, err = json.MarshalIndent(vehicles, "", "  "); err != nil {
		return
	}

	vehicleData = append(vehicleData, '\n')
	if err = os.WriteFile(filepath.Join(stagingPath, "vehicles.json"), vehicleData, 0o644); err != nil {
		return
	}

	var vehicleHash [32]byte = sha256.Sum256(vehicleData)
	files["vehicles.json"] = hex.EncodeToString(vehicleHash[:])
	var planeIconData []byte
	if planeIconData, err = json.MarshalIndent(planeIcons, "", "  "); err != nil {
		return
	}
	planeIconData = append(planeIconData, '\n')
	if err = os.WriteFile(filepath.Join(stagingPath, "plane_icons.json"), planeIconData, 0o644); err != nil {
		return
	}
	var planeIconHash [32]byte = sha256.Sum256(planeIconData)
	files["plane_icons.json"] = hex.EncodeToString(planeIconHash[:])
	if err = copyShipSilhouettes(silhouettesPath, stagingPath, vehicles, files); err != nil {
		return
	}

	if err = copyShipClassIcons(classIconsPath, stagingPath, files); err != nil {
		return
	}

	if err = copySquadronIcons(squadronIconsPath, stagingPath, files); err != nil {
		return
	}
	if err = copyPlaneIcons(planeIconsPath, stagingPath, files); err != nil {
		return
	}

	var (
		packageManifest manifest = manifest{FormatVersion: 1, ClientVersion: args.Version, Files: files}
		manifestData    []byte
	)

	if manifestData, err = json.MarshalIndent(packageManifest, "", "  "); err != nil {
		return
	}

	if err = os.WriteFile(filepath.Join(stagingPath, "manifest.json"), append(manifestData, '\n'), 0o644); err != nil {
		return
	}

	if args.Force {
		if err = os.RemoveAll(outputPath); err != nil {
			return
		}
	}

	if err = os.Rename(stagingPath, outputPath); err != nil {
		return
	}

	fmt.Printf("Extracted %d schema files for %s to %s\n", len(files), args.Version, outputPath)
	return
}

// copyPlaneIcons packages only the game's minimap aircraft marker variants.
func copyPlaneIcons(sourceDirectory string, packageDirectory string, files map[string]string) (err error) {
	err = filepath.WalkDir(sourceDirectory, func(source string, entry os.DirEntry, walkErr error) (callbackErr error) {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".png" {
			return
		}
		var relativePath string
		if relativePath, callbackErr = filepath.Rel(sourceDirectory, source); callbackErr != nil {
			return
		}
		var packagePath string = filepath.Join("plane_icons", relativePath)
		return copyFile(source, filepath.Join(packageDirectory, packagePath), packagePath, files)
	})
	return
}

// copyShipClassIcons packages the unified minimap marker and its visibility/death variants for each ship class.
func copyShipClassIcons(sourceDirectory string, packageDirectory string, files map[string]string) (err error) {
	for _, species := range []string{"aircarrier", "battleship", "cruiser", "destroyer", "submarine"} {
		for _, state := range []string{"", "_dead", "_last_visible"} {
			var (
				name        string = "minimap_" + species + state + ".svg"
				packagePath string = filepath.Join("ships_classes", species+state+".svg")
			)

			if err = copyFile(filepath.Join(sourceDirectory, name), filepath.Join(packageDirectory, packagePath), packagePath, files); err != nil {
				return
			}
		}
	}

	return
}

// copyShipSilhouettes copies only icons referenced by the extracted ship catalog.
func copyShipSilhouettes(sourceDirectory string, packageDirectory string, vehicles []replaydata.Vehicle, files map[string]string) (err error) {
	var copied map[string]bool = make(map[string]bool)
	for _, vehicle := range vehicles {
		if vehicle.ShipID == "" || filepath.Base(vehicle.ShipID) != vehicle.ShipID || copied[vehicle.ShipID] {
			continue
		}

		copied[vehicle.ShipID] = true
		var (
			name        string = vehicle.ShipID + ".png"
			sourcePath  string = filepath.Join(sourceDirectory, name)
			packagePath string = filepath.Join("ships_silhouettes", name)
		)

		if _, err = os.Stat(sourcePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				err = nil
				continue
			}

			return
		}

		if err = copyFile(sourcePath, filepath.Join(packageDirectory, packagePath), packagePath, files); err != nil {
			return
		}
	}

	return
}

// copySquadronIcons packages the game's ally and enemy minimap aircraft markers.
func copySquadronIcons(sourceDirectory string, packageDirectory string, files map[string]string) (err error) {
	for _, icon := range []struct {
		Source string
		Target string
	}{
		{Source: "aircraft.png", Target: "squadrons/aircraft.png"},
	} {
		if err = copyFile(filepath.Join(sourceDirectory, icon.Source), filepath.Join(packageDirectory, filepath.FromSlash(icon.Target)), icon.Target, files); err != nil {
			return
		}
	}

	return
}

// copyFile copies one schema file and records its SHA-256 in the package manifest.
func copyFile(source, destination, packagePath string, files map[string]string) (err error) {
	var input *os.File
	if input, err = os.Open(source); err != nil { // #nosec G304 -- source is from the selected install's WalkDir.
		return
	}

	defer input.Close()

	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return
	}

	var output *os.File
	if output, err = os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil { // #nosec G304 -- destination is a package-relative path under staging.
		return
	}

	defer output.Close()

	var digest hash.Hash = sha256.New()
	if _, err = io.Copy(io.MultiWriter(output, digest), input); err != nil {
		return
	}

	files[filepath.ToSlash(packagePath)] = hex.EncodeToString(digest.Sum(nil))
	return
}

// validVersion prevents path traversal through the version-derived output directory.
func validVersion(version string) (valid bool) {
	if version == "" || strings.HasPrefix(version, ".") || strings.Contains(version, "..") {
		valid = false
		return
	}

	for _, character := range version {
		if (character < '0' || character > '9') && character != '.' {
			valid = false
			return
		}
	}

	valid = true
	return
}
