package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gogpu/gg/text"
	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
)

const (
	maxVideoFrames   int    = 30000
	maxVideoRawBytes uint64 = 32 << 30
	// shortcut: show only recent sync positions for 2.5 seconds; extend after verified trajectory decoding.
	torpedoMarkerTTL float64 = 2.5
)

type (
	positionSample struct {
		clock    float64
		position render.Ship
	}

	torpedoID struct {
		ownerID uint32
		shotID  uint16
	}

	trackedTorpedo struct {
		marker      render.Torpedo
		lastUpdated float64
	}

	replayFrameState struct {
		events          []events.Event
		nextEvent       int
		ownShipID       uint32
		ownTeamID       int64
		ownTeamKnown    bool
		ships           map[uint32]render.Ship
		hiddenPositions map[uint32]render.Ship
		squadrons       map[int64]render.Squadron
		torpedoes       map[torpedoID]trackedTorpedo
		capturePoints   map[uint32]render.CapturePoint
		positionClocks  map[uint32]float64
		minimapVisible  map[uint32]bool
	}
)

// newReplayFrameState creates an incremental state builder for chronological video frames.
func newReplayFrameState(timeline *events.Timeline) (state *replayFrameState) {
	state = &replayFrameState{events: timeline.Events, ownShipID: timeline.OwnShipID, ships: make(map[uint32]render.Ship), hiddenPositions: make(map[uint32]render.Ship), squadrons: make(map[int64]render.Squadron), torpedoes: make(map[torpedoID]trackedTorpedo), capturePoints: make(map[uint32]render.CapturePoint), positionClocks: make(map[uint32]float64), minimapVisible: make(map[uint32]bool)}
	return
}

// bindShipLineup attaches a roster silhouette and ship label only when team and max health identify one player.
func bindShipLineup(ships []render.Ship, players []render.Player) {
	for shipIndex := range ships {
		if ships[shipIndex].MaxHealth <= 0 || !ships[shipIndex].TeamKnown {
			continue
		}

		var matchedIndex int = -1
		for playerIndex, player := range players {
			if player.TeamID == uint64(ships[shipIndex].TeamID) && math.Abs(player.MaxHealth-ships[shipIndex].MaxHealth) <= 1 {
				if matchedIndex >= 0 {
					matchedIndex = -1
					break
				}

				matchedIndex = playerIndex
			}
		}

		if matchedIndex >= 0 {
			ships[shipIndex].ShipName = players[matchedIndex].ShipName
			ships[shipIndex].Icon = players[matchedIndex].ClassIcon
			ships[shipIndex].DeadIcon = players[matchedIndex].ClassIconDead
			ships[shipIndex].UnspottedIcon = players[matchedIndex].ClassIconUnspotted
			players[matchedIndex].Health = ships[shipIndex].Health
			players[matchedIndex].Alive = !ships[shipIndex].Dead
		}
	}
}

// shortcut: decode the 15.9 D6-00 float path; use schema-indexed nested-state decoding for other client layouts.
func decodeCaptureProgress(payload []byte) (progress float64, ok bool) {
	if len(payload) != 6 || payload[0] != 0xd6 || payload[1] != 0 {
		return
	}

	progress = float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[2:])))
	if math.IsNaN(progress) || math.IsInf(progress, 0) || progress < 0 {
		progress = 0
		return
	}

	ok = true
	return
}

// ShipsAt advances vehicle state through the requested replay time and returns stable ordering.
func (state *replayFrameState) ShipsAt(clock float64) (ships []render.Ship) {
	for state.nextEvent < len(state.events) && float64(state.events[state.nextEvent].Clock) <= clock {
		var event events.Event = state.events[state.nextEvent]
		state.nextEvent++
		switch event.Type {
		case events.NestedPropertyEvent:
			if event.Member != nil {
				if point, exists := state.capturePoints[event.EntityID]; exists {
					if progress, ok := decodeCaptureProgress(event.Member.Payload); ok {
						point.Progress = progress
						state.capturePoints[event.EntityID] = point
					}
				}
			}
		case events.TorpedoPositionEvent:
			if event.Torpedo != nil {
				var marker render.Torpedo = render.Torpedo{X: float64(event.Torpedo.Position.X), Z: float64(event.Torpedo.Position.Z), OwnerID: event.Torpedo.OwnerID}
				if owner, exists := state.ships[event.Torpedo.OwnerID]; exists {
					marker.TeamID = owner.TeamID
					marker.TeamKnown = owner.TeamKnown
				}

				state.torpedoes[torpedoID{ownerID: event.Torpedo.OwnerID, shotID: event.Torpedo.ShotID}] = trackedTorpedo{marker: marker, lastUpdated: float64(event.Clock)}
			}
		case events.MinimapSquadronEvent:
			if event.Squadron != nil {
				state.squadrons[event.Squadron.PlaneID] = render.Squadron{X: float64(event.Squadron.X), Z: float64(event.Squadron.Z), TeamID: int64(event.Squadron.TeamID), TeamKnown: true}
			}
		case events.MinimapSquadronPositionEvent:
			if event.Squadron != nil {
				if squadron, exists := state.squadrons[event.Squadron.PlaneID]; exists {
					squadron.X = float64(event.Squadron.X)
					squadron.Z = float64(event.Squadron.Z)
					state.squadrons[event.Squadron.PlaneID] = squadron
				}
			}
		case events.MinimapSquadronRemoveEvent:
			if event.Squadron != nil {
				delete(state.squadrons, event.Squadron.PlaneID)
			}
		case events.MinimapShipPositionEvent:
			if event.Minimap != nil {
				if ship, exists := state.ships[event.EntityID]; exists && !ship.Dead {
					if event.Minimap.IsSentinel || event.Minimap.Disappearing {
						state.minimapVisible[event.EntityID] = false
						if ship.Enemy && ship.HasBeenSpotted {
							ship.Spotted = false
						}

						state.ships[event.EntityID] = ship
						continue
					}

					state.minimapVisible[event.EntityID] = true
					ship.X = float64(event.Minimap.X)
					ship.Z = float64(event.Minimap.Z)
					ship.Heading = float64(event.Minimap.Heading) * math.Pi / 180
					if ship.Enemy {
						ship.HasBeenSpotted = true
						ship.Spotted = true
					}

					state.ships[event.EntityID] = ship
					state.positionClocks[event.EntityID] = float64(event.Clock)
				}
			}
		case events.EntityCreateEvent:
			if event.Created != nil && event.Created.Position != nil {
				if event.Created.EntityType == "Vehicle" {
					var ship render.Ship = render.Ship{X: float64(event.Created.Position.X), Z: float64(event.Created.Position.Z), Heading: float64(event.Created.Position.Yaw), Spotted: true}
					if event.Created.InitialProperties != nil {
						if teamID, ok := event.Created.InitialProperties["teamId"].(int8); ok {
							ship.TeamID = int64(teamID)
							ship.TeamKnown = true
						}

						if alive, ok := event.Created.InitialProperties["isAlive"].(bool); ok {
							ship.Dead = !alive
						}

						if health, ok := event.Created.InitialProperties["health"].(float32); ok {
							ship.Health = float64(health)
						}

						if maxHealth, ok := event.Created.InitialProperties["maxHealth"].(float32); ok {
							ship.MaxHealth = float64(maxHealth)
						}
					}

					state.ships[event.EntityID] = ship
					state.positionClocks[event.EntityID] = float64(event.Clock)
					if event.EntityID == state.ownShipID && ship.TeamKnown {
						state.ownTeamID = ship.TeamID
						state.ownTeamKnown = true
					}

					state.classifyShips()
				} else if event.Created.EntityType == "InteractiveZone" && event.Clock <= 1 {
					var point render.CapturePoint = render.CapturePoint{X: float64(event.Created.Position.X), Z: float64(event.Created.Position.Z)}
					if event.Created.InitialProperties != nil {
						if radius, ok := event.Created.InitialProperties["radius"].(float32); ok {
							point.Radius = float64(radius)
						}

						if teamID, ok := event.Created.InitialProperties["teamId"].(int8); ok && teamID >= 0 {
							point.TeamID = int64(teamID)
							point.TeamKnown = true
						}
					}

					state.capturePoints[event.EntityID] = point
				}
			}
		case events.PositionEvent, events.NonVolatilePositionEvent:
			if event.Position != nil {
				if ship, exists := state.ships[event.EntityID]; exists && !ship.Dead {
					var position render.Ship = render.Ship{X: float64(event.Position.X), Z: float64(event.Position.Z), Heading: float64(event.Position.Yaw)}
					if ship.Enemy && !ship.Spotted {
						state.hiddenPositions[event.EntityID] = position
					} else if ship.Spotted {
						ship.X = position.X
						ship.Z = position.Z
						ship.Heading = position.Heading
						state.ships[event.EntityID] = ship
						state.positionClocks[event.EntityID] = float64(event.Clock)
					}
				}
			}
		case events.PlayerOrientationEvent:
			if event.EntityID == state.ownShipID && event.Position != nil {
				if ship, exists := state.ships[event.EntityID]; exists && !ship.Dead {
					ship.X = float64(event.Position.X)
					ship.Z = float64(event.Position.Z)
					ship.Heading = float64(event.Position.Yaw)
					state.ships[event.EntityID] = ship
					state.positionClocks[event.EntityID] = float64(event.Clock)
				}
			}
		case events.EntityLeaveEvent:
			if ship, exists := state.ships[event.EntityID]; exists && !ship.Dead && ship.Enemy {
				state.hiddenPositions[event.EntityID] = render.Ship{X: ship.X, Z: ship.Z, Heading: ship.Heading}
				if !state.minimapVisible[event.EntityID] {
					ship.Spotted = false
				}

				ship.VisibilityKnown = true
				state.ships[event.EntityID] = ship
			}
		case events.ShipVisibilityEvent:
			if event.Visibility != nil {
				var spottedIDs map[uint32]bool = make(map[uint32]bool, len(event.Visibility.EntityIDs))
				for _, entityID := range event.Visibility.EntityIDs {
					spottedIDs[entityID] = true
				}

				for entityID, ship := range state.ships {
					ship.VisibilityKnown = true
					if !ship.Dead {
						if spottedIDs[entityID] && entityID != state.ownShipID {
							if !ship.TeamKnown {
								ship.Enemy = true
							}

							if ship.Enemy {
								ship.HasBeenSpotted = true
							}
						}

						if _, hasMinimapState := state.minimapVisible[entityID]; !ship.Enemy || !hasMinimapState {
							ship.Spotted = !ship.Enemy || spottedIDs[entityID]
						}

						if ship.Enemy && ship.Spotted {
							if lastPosition, exists := state.hiddenPositions[entityID]; exists {
								ship.X = lastPosition.X
								ship.Z = lastPosition.Z
								ship.Heading = lastPosition.Heading
								state.positionClocks[entityID] = float64(event.Clock)
							}
						}

						state.ships[entityID] = ship
					}
				}
			}
		case events.ShipHealthEvent:
			if event.Health != nil {
				if ship, exists := state.ships[event.EntityID]; exists {
					if event.Health.Current <= 0 {
						ship.Health = 0
						ship.Dead = true
					} else if !ship.Enemy || ship.Spotted {
						ship.Health = float64(event.Health.Current)
					}

					state.ships[event.EntityID] = ship
				}
			}
		case events.EntityPropertyEvent:
			if event.Member != nil {
				if point, exists := state.capturePoints[event.EntityID]; exists && event.Member.EntityType == "InteractiveZone" && event.Member.Name == "teamId" && len(event.Member.Payload) == 1 {
					var teamID int8 = int8(event.Member.Payload[0])
					point.TeamID = int64(teamID)
					point.TeamKnown = teamID >= 0
					point.Enemy = state.ownTeamKnown && point.TeamKnown && point.TeamID != state.ownTeamID
					state.capturePoints[event.EntityID] = point
				}

				if ship, exists := state.ships[event.EntityID]; exists && event.Member.EntityType == "Vehicle" {
					switch event.Member.Name {
					case "isAlive":
						if len(event.Member.Payload) == 1 {
							ship.Dead = event.Member.Payload[0] == 0
						}
					case "teamId":
						if len(event.Member.Payload) == 1 {
							ship.TeamID = int64(int8(event.Member.Payload[0]))
							ship.TeamKnown = true
							if event.EntityID == state.ownShipID {
								state.ownTeamID = ship.TeamID
								state.ownTeamKnown = true
							}
						}
					}

					state.ships[event.EntityID] = ship
					state.classifyShips()
				}

				if point, exists := state.capturePoints[event.EntityID]; exists {
					point.Enemy = state.ownTeamKnown && point.TeamKnown && point.TeamID != state.ownTeamID
					state.capturePoints[event.EntityID] = point
				}
			}
		}
	}

	var shipIDs []uint32
	for entityID := range state.ships {
		shipIDs = append(shipIDs, entityID)
	}

	sort.Slice(shipIDs, func(left int, right int) bool {
		return shipIDs[left] < shipIDs[right]
	})

	for _, entityID := range shipIDs {
		var ship render.Ship = state.ships[entityID]
		if !ship.Dead && (!ship.Enemy || ship.Spotted) {
			state.interpolateShip(entityID, clock, &ship)
		}

		ship.Self = entityID == state.ownShipID
		if ship.Self {
			ship.Team = 0
		} else if ship.TeamKnown && state.ownTeamKnown && ship.TeamID == state.ownTeamID {
			ship.Team = 1
		} else if ship.TeamKnown && state.ownTeamKnown {
			ship.Team = 2
		} else if ship.Enemy {
			ship.Team = 2
		} else {
			ship.Team = 1
		}

		ships = append(ships, ship)
	}
	return
}

// interpolateShip smooths visible ship movement between nearby authoritative samples.
func (state *replayFrameState) interpolateShip(entityID uint32, clock float64, ship *render.Ship) {
	const maxInterpolationGap float64 = 2

	currentClock, exists := state.positionClocks[entityID]
	if !exists || clock < currentClock || clock-currentClock >= maxInterpolationGap {
		return
	}

	var (
		next        *positionSample
		minimapOnly bool = ship.Enemy && !ship.Spotted
	)

	for _, event := range state.events[state.nextEvent:] {
		if ship.Enemy && !state.minimapVisible[entityID] && event.Type == events.ShipVisibilityEvent && event.Visibility != nil {
			minimapOnly = !containsEntity(event.Visibility.EntityIDs, entityID)
			continue
		}

		if event.Type == events.EntityLeaveEvent && event.EntityID == entityID {
			minimapOnly = true
			continue
		}

		if event.EntityID != entityID {
			continue
		}

		var sample render.Ship
		if event.Type == events.MinimapShipPositionEvent && event.Minimap != nil && !event.Minimap.IsSentinel && !event.Minimap.Disappearing {
			sample = render.Ship{X: float64(event.Minimap.X), Z: float64(event.Minimap.Z), Heading: float64(event.Minimap.Heading) * math.Pi / 180}
		} else if !minimapOnly && event.Position != nil && (event.Type == events.PositionEvent || event.Type == events.NonVolatilePositionEvent || event.Type == events.PlayerOrientationEvent) {
			sample = render.Ship{X: float64(event.Position.X), Z: float64(event.Position.Z), Heading: float64(event.Position.Yaw)}
		} else {
			continue
		}

		if float64(event.Clock)-currentClock > maxInterpolationGap {
			return
		}

		next = &positionSample{clock: float64(event.Clock), position: sample}
		break
	}

	if next == nil || next.clock <= currentClock {
		return
	}

	var fraction float64 = (clock - currentClock) / (next.clock - currentClock)
	ship.X += (next.position.X - ship.X) * fraction
	ship.Z += (next.position.Z - ship.Z) * fraction
	ship.Heading += math.Atan2(math.Sin(next.position.Heading-ship.Heading), math.Cos(next.position.Heading-ship.Heading)) * fraction
}

// containsEntity reports whether a visibility update includes the requested ship.
func containsEntity(entityIDs []uint32, entityID uint32) (contains bool) {
	for _, candidate := range entityIDs {
		if candidate == entityID {
			contains = true
			return
		}
	}

	return
}

// classifyShips applies the replay's team IDs once the player's team is known.
func (state *replayFrameState) classifyShips() {
	if !state.ownTeamKnown {
		return
	}

	for entityID, ship := range state.ships {
		if ship.TeamKnown && entityID != state.ownShipID {
			ship.Enemy = ship.TeamID != state.ownTeamID
			if minimapVisible, hasMinimapState := state.minimapVisible[entityID]; hasMinimapState {
				ship.Spotted = minimapVisible
			} else if !ship.VisibilityKnown {
				ship.Spotted = !ship.Enemy
			}

			if ship.Enemy && ship.Spotted {
				ship.HasBeenSpotted = true
			}

			state.ships[entityID] = ship
		}
	}
}

// CapturePoints returns objective positions discovered in the replay's map entities.
func (state *replayFrameState) CapturePoints() (points []render.CapturePoint) {
	var entityIDs []uint32
	for entityID := range state.capturePoints {
		entityIDs = append(entityIDs, entityID)
	}

	sort.Slice(entityIDs, func(left int, right int) bool {
		return entityIDs[left] < entityIDs[right]
	})

	for index, entityID := range entityIDs {
		var point render.CapturePoint = state.capturePoints[entityID]
		point.Label = string(rune('A' + index))
		point.Enemy = state.ownTeamKnown && point.TeamKnown && point.TeamID != state.ownTeamID
		points = append(points, point)
	}

	return
}

// SquadronsAt returns active minimap aircraft in stable ID order and relative team colors.
func (state *replayFrameState) SquadronsAt() (squadrons []render.Squadron) {
	var planeIDs []int64
	for planeID := range state.squadrons {
		planeIDs = append(planeIDs, planeID)
	}

	sort.Slice(planeIDs, func(left int, right int) bool {
		return planeIDs[left] < planeIDs[right]
	})

	for _, planeID := range planeIDs {
		var squadron render.Squadron = state.squadrons[planeID]
		squadron.Enemy = squadron.TeamKnown && state.ownTeamKnown && squadron.TeamID != state.ownTeamID
		squadrons = append(squadrons, squadron)
	}

	return
}

// TorpedoesAt returns recently synchronized torpedoes with team colors resolved from their owner ships.
func (state *replayFrameState) TorpedoesAt(clock float64) (torpedoes []render.Torpedo) {
	var torpedoIDs []torpedoID
	for id := range state.torpedoes {
		torpedoIDs = append(torpedoIDs, id)
	}

	sort.Slice(torpedoIDs, func(left int, right int) bool {
		if torpedoIDs[left].ownerID == torpedoIDs[right].ownerID {
			return torpedoIDs[left].shotID < torpedoIDs[right].shotID
		}

		return torpedoIDs[left].ownerID < torpedoIDs[right].ownerID
	})

	for _, id := range torpedoIDs {
		var tracked trackedTorpedo = state.torpedoes[id]
		if clock-tracked.lastUpdated > torpedoMarkerTTL {
			delete(state.torpedoes, id)
			continue
		}

		if owner, exists := state.ships[tracked.marker.OwnerID]; exists {
			tracked.marker.TeamID = owner.TeamID
			tracked.marker.TeamKnown = owner.TeamKnown
		}

		tracked.marker.Enemy = tracked.marker.TeamKnown && state.ownTeamKnown && tracked.marker.TeamID != state.ownTeamID
		torpedoes = append(torpedoes, tracked.marker)
	}

	return
}

// exportReplayVideo renders sampled replay frames and streams them to FFmpeg without temporary frame files.
func exportReplayVideo(args arguments, replay *replayparser.Replay, timeline *events.Timeline, schema *replaydata.Schema) (err error) {
	var (
		mapName  string
		mapImage image.Image
		bounds   render.Bounds
		font     text.Face
	)

	if mapName, mapImage, bounds, font, err = loadRenderResources(args, replay); err != nil {
		return
	}

	args.At = 0
	var frame render.Frame
	if frame, err = buildFrame(args, replay, timeline, mapImage, mapName, bounds, font, schema); err != nil {
		return
	}

	frame.Layout.Width = args.VideoWidth
	frame.Layout.Height = args.VideoHeight
	frame.Layout.LeftPanelWidth = args.VideoWidth * 280 / 1328
	frame.Layout.RightPanelWidth = args.VideoWidth * 280 / 1328
	frame.Layout.FooterHeight = args.VideoHeight * 48 / 800
	if frame.Duration <= 0 {
		err = fmt.Errorf("replay has no positive battle duration")
		return
	}

	var frameCount int
	if frameCount, err = videoFrameCount(frame.Duration, args.VideoFPS, args.VideoSpeed, frame.Layout.Width, frame.Layout.Height); err != nil {
		return
	}

	var encoderPath string
	if encoderPath, err = exec.LookPath(args.FFmpegPath); err != nil {
		err = fmt.Errorf("find FFmpeg executable: %w", err)
		return
	}

	if err = os.MkdirAll(filepath.Dir(args.ExportVideo), 0o755); err != nil {
		return
	}

	var reserved *os.File
	if reserved, err = os.OpenFile(args.ExportVideo, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
		return
	}

	if err = reserved.Close(); err != nil {
		os.Remove(args.ExportVideo) // #nosec G104 -- cleanup must not replace the close error.
		return
	}

	var (
		completed     bool
		temporaryPath string
	)

	defer func() {
		if temporaryPath != "" {
			os.Remove(temporaryPath) // #nosec G104 -- best-effort cleanup; primary export error is returned.
		}

		if !completed {
			os.Remove(args.ExportVideo) // #nosec G104 -- best-effort cleanup; primary export error is returned.
		}
	}()

	var temporary *os.File
	if temporary, err = os.CreateTemp(filepath.Dir(args.ExportVideo), ".wows-replay-*.mp4"); err != nil {
		return
	}

	temporaryPath = temporary.Name()
	if err = temporary.Close(); err != nil {
		return
	}

	var (
		stderr  bytes.Buffer
		rgba    *image.RGBA = image.NewRGBA(image.Rect(0, 0, frame.Layout.Width, frame.Layout.Height))
		command *exec.Cmd   = exec.Command(encoderPath, // #nosec G204 -- executable is user-selected; fixed args are passed without a shell.
			"-hide_banner", "-loglevel", "error", "-nostats", "-y",
			"-f", "rawvideo", "-pixel_format", "rgba", "-video_size", fmt.Sprintf("%dx%d", frame.Layout.Width, frame.Layout.Height),
			"-framerate", strconv.Itoa(args.VideoFPS), "-i", "pipe:0", "-an", "-c:v", "libx264", "-preset", "veryfast",
			"-crf", strconv.Itoa(args.VideoCRF), "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-f", "mp4", temporaryPath,
		)
	)

	command.Stderr = &stderr

	var input io.WriteCloser
	if input, err = command.StdinPipe(); err != nil {
		return
	}

	if err = command.Start(); err != nil {
		input.Close() // #nosec G104 -- command did not start; close only releases the pipe.
		return
	}

	var state *replayFrameState = newReplayFrameState(timeline)
	for index := range frameCount {
		var clock float64 = math.Min(frame.Duration, float64(index)*args.VideoSpeed/float64(args.VideoFPS))
		frame.Clock = clock
		frame.Ships = state.ShipsAt(clock)
		bindShipLineup(frame.Ships, frame.LeftTeam)
		bindShipLineup(frame.Ships, frame.RightTeam)
		frame.Squadrons = state.SquadronsAt()
		frame.Torpedoes = state.TorpedoesAt(clock)
		frame.CapturePoints = state.CapturePoints()

		var imageFrame image.Image
		if imageFrame, err = render.RenderFrame(frame); err != nil {
			break
		}

		if err = writeRGBAFrame(input, imageFrame, rgba); err != nil {
			break
		}
	}

	if err != nil {
		input.Close()          // #nosec G104 -- preserve the frame-render/write error during encoder shutdown.
		command.Process.Kill() // #nosec G104 -- preserve the frame-render/write error during encoder shutdown.
		command.Wait()         // #nosec G104 -- preserve the frame-render/write error during encoder shutdown.
		return
	}

	if err = input.Close(); err != nil {
		command.Process.Kill() // #nosec G104 -- preserve the pipe-close error during encoder shutdown.
		command.Wait()         // #nosec G104 -- preserve the pipe-close error during encoder shutdown.
		return
	}

	if err = command.Wait(); err != nil {
		err = fmt.Errorf("FFmpeg video encoding failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		return
	}

	if err = os.Rename(temporaryPath, args.ExportVideo); err != nil {
		return
	}

	completed = true
	fmt.Printf("Rendered %.1f seconds of replay at %.1fx to %s\n", frame.Duration, args.VideoSpeed, args.ExportVideo)
	return
}

// videoFrameCount bounds both frame count and estimated raw RGBA work before launching FFmpeg.
func videoFrameCount(duration float64, fps int, speed float64, width int, height int) (frameCount int, err error) {
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) || fps < 1 || speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) || width <= 0 || height <= 0 {
		err = fmt.Errorf("video duration, rate, speed, or dimensions are invalid")
		return
	}

	var requestedFrames float64 = math.Ceil(duration*float64(fps)/speed) + 1
	if math.IsInf(requestedFrames, 0) || requestedFrames > float64(maxVideoFrames) {
		err = fmt.Errorf("video would require %.0f frames, exceeding the safe limit of %d", requestedFrames, maxVideoFrames)
		return
	}

	frameCount = int(requestedFrames)

	if uint64(width) > maxVideoRawBytes/4/uint64(height) {
		err = fmt.Errorf("video frame dimensions exceed the safe raw-frame budget")
		return
	}

	var bytesPerFrame uint64 = uint64(width) * uint64(height) * 4
	if uint64(frameCount) > maxVideoRawBytes/bytesPerFrame {
		err = fmt.Errorf("video would stream more than %d GiB of raw frames; reduce resolution, frame rate, or match duration", maxVideoRawBytes>>30)
	}

	return
}

// writeRGBAFrame converts and writes one tightly packed RGBA frame to FFmpeg's rawvideo input.
func writeRGBAFrame(writer io.Writer, frame image.Image, pixels *image.RGBA) (err error) {
	draw.Draw(pixels, pixels.Bounds(), frame, frame.Bounds().Min, draw.Src)

	var offset int
	for offset < len(pixels.Pix) {
		var written int
		if written, err = writer.Write(pixels.Pix[offset:]); err != nil {
			return
		}

		if written == 0 {
			err = io.ErrShortWrite
			return
		}

		offset += written
	}

	return
}
