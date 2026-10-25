package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
)

const (
	maxVideoFrames     int     = 30000
	maxVideoRawBytes   uint64  = 32 << 30
	victoryPointsScale float64 = 100
)

type (
	// Timeline is the decoded event sequence consumed by the replay video renderer.
	Timeline = events.Timeline
	// Frame is a complete, display-independent replay frame description.
	Frame = render.Frame

	// Options configures sampled frames and optional FFmpeg MP4 encoding.
	Options struct {
		OutputPath string
		FFmpegPath string
		FPS        int
		Speed      float64
		CRF        int
		Scale      float64
		Workers    int
	}

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
		directionX  float64
		directionZ  float64
		lastUpdated float64
		launchedAt  float64
	}

	trackedShell struct {
		shot           events.ArtilleryShot
		firedAt        float64
		flightDuration float64
	}

	frameRenderJob struct {
		index int
		frame render.Frame
	}

	frameRenderResult struct {
		index int
		image image.Image
		err   error
	}

	scoreSample struct {
		clock  float64
		points float64
	}

	FrameState struct {
		events          []events.Event
		nextEvent       int
		ownShipID       uint32
		ownTeamID       int64
		ownTeamKnown    bool
		ships           map[uint32]render.Ship
		hiddenPositions map[uint32]render.Ship
		squadrons       map[int64]render.Squadron
		torpedoes       map[torpedoID]trackedTorpedo
		shells          map[torpedoID]trackedShell
		capturePoints   map[uint32]render.CapturePoint
		startingPoints  float64
		ownPoints       float64
		ownScoreKnown   bool
		ownPointsPerSec float64
		scoreSamples    []scoreSample
		teamPoints      map[int64]float64
		teamRates       map[int64]float64
		teamSamples     map[int64][]scoreSample
		results         map[uint64]events.BattlePlayerResult
		kills           map[uint32]float64
		damage          map[uint32]float64
		positionClocks  map[uint32]float64
		minimapVisible  map[uint32]bool
	}
)

// NewFrameState creates an incremental state builder for chronological replay frames.
func NewFrameState(timeline *Timeline) (state *FrameState) {
	state = &FrameState{events: timeline.Events, ownShipID: timeline.OwnShipID, ships: make(map[uint32]render.Ship), hiddenPositions: make(map[uint32]render.Ship), squadrons: make(map[int64]render.Squadron), torpedoes: make(map[torpedoID]trackedTorpedo), shells: make(map[torpedoID]trackedShell), capturePoints: make(map[uint32]render.CapturePoint), positionClocks: make(map[uint32]float64), minimapVisible: make(map[uint32]bool), teamPoints: make(map[int64]float64), teamRates: make(map[int64]float64), teamSamples: make(map[int64][]scoreSample), results: make(map[uint64]events.BattlePlayerResult), kills: make(map[uint32]float64), damage: make(map[uint32]float64)}
	return
}

// FrameAt advances replay state serially and snapshots mutable sidebar slices for concurrent rendering.
func (state *FrameState) FrameAt(base Frame, clock float64) (frame Frame) {
	frame = base
	state.initializeScore(base.LeftStartingPoints)
	frame.Clock = clock
	frame.Ships = state.ShipsAt(clock)
	frame.LeftTeam = append([]render.Player(nil), base.LeftTeam...)
	frame.RightTeam = append([]render.Player(nil), base.RightTeam...)
	BindShipLineup(frame.Ships, frame.LeftTeam)
	BindShipLineup(frame.Ships, frame.RightTeam)
	state.applyLiveStats(frame.LeftTeam)
	state.applyLiveStats(frame.RightTeam)
	state.applyResultStats(frame.LeftTeam)
	state.applyResultStats(frame.RightTeam)
	render.SortPlayers(frame.LeftTeam)
	render.SortPlayers(frame.RightTeam)
	frame.Squadrons = state.SquadronsAt()
	for index := range frame.Squadrons {
		frame.Squadrons[index].IconName = base.PlaneIconNames[frame.Squadrons[index].GameParamsID]
	}
	frame.Torpedoes = state.TorpedoesAt(clock)
	frame.Shells = state.ShellsAt(clock)
	frame.CapturePoints = state.CapturePoints()
	frame.LeftTeamPoints = base.LeftStartingPoints
	frame.RightTeamPoints = base.RightStartingPoints
	frame.LeftPointsPerSec = 0
	frame.RightPointsPerSec = 0
	if state.ownScoreKnown {
		frame.LeftTeamPoints = state.ownPoints
		frame.LeftPointsPerSec = state.ownPointsPerSec
	}
	if points, exists := state.teamPoints[state.ownTeamID]; state.ownTeamKnown && exists {
		frame.LeftTeamPoints = points
		frame.LeftPointsPerSec = state.teamRates[state.ownTeamID]
	}
	for teamID, points := range state.teamPoints {
		if state.ownTeamKnown && teamID != state.ownTeamID {
			frame.RightTeamPoints = points
			frame.RightPointsPerSec = state.teamRates[teamID]
			break
		}
	}
	return
}

// renderWorkerCount uses all available processors by default and honors explicit worker counts.
func renderWorkerCount(requested int) (count int, err error) {
	if requested < 0 {
		err = fmt.Errorf("video render workers must not be negative")
		return
	}

	count = requested
	if count == 0 {
		count = runtime.GOMAXPROCS(0)
	}

	if count < 1 {
		count = 1
	}
	return
}

// bindShipLineup attaches a roster silhouette and ship label only when team and max health identify one player.
// BindShipLineup associates replay vehicle markers with lineup names and icons.
func BindShipLineup(ships []render.Ship, players []render.Player) {
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
			players[matchedIndex].EntityID = ships[shipIndex].EntityID
			ships[shipIndex].ShipName = players[matchedIndex].ShipName
			ships[shipIndex].Icon = players[matchedIndex].ClassIcon
			ships[shipIndex].DeadIcon = players[matchedIndex].ClassIconDead
			ships[shipIndex].UnspottedIcon = players[matchedIndex].ClassIconUnspotted
			players[matchedIndex].Health = ships[shipIndex].Health
			players[matchedIndex].Alive = !ships[shipIndex].Dead
			players[matchedIndex].Detected = ships[shipIndex].Enemy && ships[shipIndex].Spotted || !ships[shipIndex].Enemy && ships[shipIndex].DetectedByEnemy
		}
	}
}

// applyLiveStats exposes damage and kill totals as combat packets arrive.
func (state *FrameState) applyLiveStats(players []render.Player) {
	for playerIndex := range players {
		players[playerIndex].Kills = state.kills[players[playerIndex].EntityID]
		players[playerIndex].Damage = state.damage[players[playerIndex].EntityID]
	}
}

// applyResultStats reveals battle-result totals only when their replay event has occurred.
func (state *FrameState) applyResultStats(players []render.Player) {
	for playerIndex := range players {
		if result, exists := state.results[players[playerIndex].AccountDBID]; exists {
			players[playerIndex].Damage = result.Damage
			players[playerIndex].Kills = result.ShipsKilled
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
func (state *FrameState) ShipsAt(clock float64) (ships []render.Ship) {
	for state.nextEvent < len(state.events) && float64(state.events[state.nextEvent].Clock) <= clock {
		var event events.Event = state.events[state.nextEvent]
		state.nextEvent++
		switch event.Type {
		case events.ShipDamageDealtEvent:
			if event.DamageDealt != nil {
				state.damage[event.DamageDealt.AggressorID] += float64(event.DamageDealt.Damage)
			}
		case events.ShipDeathEvent:
			if event.Death != nil {
				if ship, exists := state.ships[event.Death.VictimID]; exists {
					ship.Dead = true
					ship.Health = 0
					state.ships[event.Death.VictimID] = ship
				}
				if event.Death.KillerID != 0 {
					state.kills[event.Death.KillerID]++
				}
			}
		case events.BattleResultsEvent:
			if event.Results != nil && event.Results.Summary != nil {
				for accountID, result := range event.Results.Summary.Players {
					state.results[accountID] = result
				}
			}
		case events.TeamScoreEvent:
			if event.TeamScore != nil {
				state.updateTeamScore(event.TeamScore.TeamIndex, float64(event.TeamScore.Score), float64(event.Clock))
			}
		case events.VictoryPointsEvent:
			if event.VictoryPoints != nil {
				state.updateOwnScore(float64(event.VictoryPoints.TotalPoints)/victoryPointsScale, float64(event.Clock))
			}
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

				var id torpedoID = torpedoID{ownerID: event.Torpedo.OwnerID, shotID: event.Torpedo.ShotID}
				tracked, exists := state.torpedoes[id]
				if !exists {
					tracked.launchedAt = float64(event.Clock)
				}
				tracked.marker = marker
				tracked.lastUpdated = float64(event.Clock)
				state.torpedoes[id] = tracked
			}
		case events.TorpedoLaunchEvent:
			if event.TorpedoLaunch != nil {
				var (
					launch events.TorpedoLaunch = *event.TorpedoLaunch
					marker render.Torpedo       = render.Torpedo{X: float64(launch.Position.X), Z: float64(launch.Position.Z), OwnerID: launch.OwnerID}
				)
				if owner, exists := state.ships[launch.OwnerID]; exists {
					marker.TeamID = owner.TeamID
					marker.TeamKnown = owner.TeamKnown
				}
				state.torpedoes[torpedoID{ownerID: launch.OwnerID, shotID: launch.ShotID}] = trackedTorpedo{marker: marker, directionX: float64(launch.Direction.X), directionZ: float64(launch.Direction.Z), lastUpdated: float64(event.Clock), launchedAt: float64(event.Clock)}
			}
		case events.TorpedoDirectionEvent:
			if event.TorpedoDirection != nil {
				var (
					direction events.TorpedoDirection = *event.TorpedoDirection
					id        torpedoID               = torpedoID{ownerID: direction.OwnerID, shotID: direction.ShotID}
					tracked   trackedTorpedo          = state.torpedoes[id]
					speed     float64                 = math.Hypot(tracked.directionX, tracked.directionZ) * float64(direction.SpeedCoef)
				)
				tracked.marker.X = float64(direction.Position.X)
				tracked.marker.Z = float64(direction.Position.Z)
				if math.Abs(float64(direction.TargetYaw)-2*math.Pi) > 0.01 {
					tracked.directionX = speed * math.Sin(float64(direction.TargetYaw))
					tracked.directionZ = speed * math.Cos(float64(direction.TargetYaw))
				} else if direction.SpeedCoef != 1 {
					tracked.directionX *= float64(direction.SpeedCoef)
					tracked.directionZ *= float64(direction.SpeedCoef)
				}
				tracked.lastUpdated = float64(event.Clock)
				state.torpedoes[id] = tracked
			}
		case events.ArtilleryShotEvent:
			if event.Shell != nil {
				var shot events.ArtilleryShot = *event.Shell
				var distance float64 = math.Hypot(float64(shot.Target.X-shot.Origin.X), float64(shot.Target.Z-shot.Origin.Z))
				var duration float64 = float64(shot.ServerTimeLeft)
				if duration <= 0 && shot.Speed > 0 {
					duration = distance / float64(shot.Speed)
				}
				if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
					duration = 3
				}
				if duration > 30 {
					duration = 30
				}
				state.shells[torpedoID{ownerID: shot.OwnerID, shotID: shot.ShotID}] = trackedShell{shot: shot, firedAt: float64(event.Clock), flightDuration: duration}
			}
		case events.ProjectileHitEvent:
			if event.ProjectileHit != nil {
				var id torpedoID = torpedoID{ownerID: event.ProjectileHit.OwnerID, shotID: event.ProjectileHit.ShotID}
				delete(state.torpedoes, id)
				delete(state.shells, id)
			}
		case events.MinimapSquadronEvent:
			if event.Squadron != nil {
				state.squadrons[event.Squadron.PlaneID] = render.Squadron{X: float64(event.Squadron.X), Z: float64(event.Squadron.Z), TeamID: int64(event.Squadron.TeamID), TeamKnown: true, GameParamsID: event.Squadron.GameParamsID}
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
						if visibilityFlags, ok := event.Created.InitialProperties["visibilityFlags"].(uint32); ok {
							ship.DetectedByEnemy = visibilityFlags != 0
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
					case "visibilityFlags":
						if len(event.Member.Payload) == 4 {
							ship.DetectedByEnemy = binary.LittleEndian.Uint32(event.Member.Payload) != 0
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

		ship.EntityID = entityID
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

// initializeScore sets the known match-start score before replay score updates arrive.
func (state *FrameState) initializeScore(points float64) {
	if state.scoreSamples != nil {
		return
	}

	state.startingPoints = points
	state.ownPoints = points
	state.scoreSamples = append(state.scoreSamples, scoreSample{clock: 0, points: points})
}

// updateTeamScore tracks authoritative score totals and their recent scoring rates.
func (state *FrameState) updateTeamScore(teamIndex int64, points float64, clock float64) {
	samples := state.teamSamples[teamIndex]
	if len(samples) > 0 && clock > samples[len(samples)-1].clock {
		samples = append(samples, scoreSample{clock: clock, points: points})
		for len(samples) > 1 && samples[0].clock < clock-30 {
			samples = samples[1:]
		}
		if elapsed := clock - samples[0].clock; elapsed > 0 {
			state.teamRates[teamIndex] = math.Max(0, (points-samples[0].points)/elapsed)
		}
	} else if len(samples) == 0 {
		samples = append(samples, scoreSample{clock: clock, points: points})
	}
	state.teamSamples[teamIndex] = samples
	state.teamPoints[teamIndex] = math.Min(render.WinningScore, points)
}

// updateOwnScore stores the authoritative player-team score and its recent rate.
func (state *FrameState) updateOwnScore(points float64, clock float64) {
	points += state.startingPoints
	if state.ownScoreKnown && clock > 0 {
		var previous scoreSample = state.scoreSamples[len(state.scoreSamples)-1]
		if clock > previous.clock {
			state.scoreSamples = append(state.scoreSamples, scoreSample{clock: clock, points: points})
			for len(state.scoreSamples) > 1 && state.scoreSamples[0].clock < clock-30 {
				state.scoreSamples = state.scoreSamples[1:]
			}
			var first scoreSample = state.scoreSamples[0]
			if elapsed := clock - first.clock; elapsed > 0 {
				state.ownPointsPerSec = math.Max(0, (points-first.points)/elapsed)
			}
		}
	} else {
		state.scoreSamples = append(state.scoreSamples, scoreSample{clock: clock, points: points})
	}
	state.ownPoints = math.Min(render.WinningScore, points)
	state.ownScoreKnown = true
}

// interpolateShip smooths visible ship movement between nearby authoritative samples.
func (state *FrameState) interpolateShip(entityID uint32, clock float64, ship *render.Ship) {
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
func (state *FrameState) classifyShips() {
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
func (state *FrameState) CapturePoints() (points []render.CapturePoint) {
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
func (state *FrameState) SquadronsAt() (squadrons []render.Squadron) {
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
func (state *FrameState) TorpedoesAt(clock float64) (torpedoes []render.Torpedo) {
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
		if tracked.launchedAt > clock || tracked.lastUpdated > clock {
			continue
		}
		if clock-tracked.launchedAt > 120 {
			delete(state.torpedoes, id)
			continue
		}
		tracked.marker.X += tracked.directionX * math.Max(0, clock-tracked.lastUpdated)
		tracked.marker.Z += tracked.directionZ * math.Max(0, clock-tracked.lastUpdated)
		if owner, exists := state.ships[tracked.marker.OwnerID]; exists {
			tracked.marker.TeamID = owner.TeamID
			tracked.marker.TeamKnown = owner.TeamKnown
		}

		tracked.marker.Enemy = tracked.marker.TeamKnown && state.ownTeamKnown && tracked.marker.TeamID != state.ownTeamID
		torpedoes = append(torpedoes, tracked.marker)
	}

	return
}

// ShellsAt returns the visible moving tracer segment for every shell still in flight.
func (state *FrameState) ShellsAt(clock float64) (shells []render.ShellTracer) {
	var shotIDs []torpedoID
	for id := range state.shells {
		shotIDs = append(shotIDs, id)
	}
	sort.Slice(shotIDs, func(left int, right int) bool {
		if shotIDs[left].ownerID == shotIDs[right].ownerID {
			return shotIDs[left].shotID < shotIDs[right].shotID
		}
		return shotIDs[left].ownerID < shotIDs[right].ownerID
	})

	for _, id := range shotIDs {
		var tracked trackedShell = state.shells[id]
		var elapsed float64 = clock - tracked.firedAt
		if elapsed < 0 {
			continue
		}
		if elapsed > tracked.flightDuration {
			delete(state.shells, id)
			continue
		}

		var (
			fraction float64            = math.Min(1, elapsed/tracked.flightDuration)
			length   float64            = math.Max(0, fraction-0.08)
			dx       float64            = float64(tracked.shot.Target.X - tracked.shot.Origin.X)
			dz       float64            = float64(tracked.shot.Target.Z - tracked.shot.Origin.Z)
			tailX    float64            = float64(tracked.shot.Origin.X) + dx*length
			tailZ    float64            = float64(tracked.shot.Origin.Z) + dz*length
			headX    float64            = float64(tracked.shot.Origin.X) + dx*fraction
			headZ    float64            = float64(tracked.shot.Origin.Z) + dz*fraction
			tracer   render.ShellTracer = render.ShellTracer{TailX: tailX, TailZ: tailZ, HeadX: headX, HeadZ: headZ}
		)
		if owner, exists := state.ships[tracked.shot.OwnerID]; exists {
			tracer.TeamKnown = owner.TeamKnown
			tracer.Enemy = owner.Enemy
			tracer.Self = owner.Self
		}
		shells = append(shells, tracer)
	}

	return
}

// ExportMP4 renders chronological frames and streams them to FFmpeg without temporary frame files.
func ExportMP4(timeline *Timeline, baseFrame Frame, options Options) (err error) {
	return ExportMP4Context(context.Background(), timeline, baseFrame, options)
}

// ExportMP4Context exports an MP4 and stops promptly when its context is canceled.
func ExportMP4Context(ctx context.Context, timeline *Timeline, baseFrame Frame, options Options) (err error) {
	if ctx == nil {
		err = fmt.Errorf("video export context is required")
		return
	}
	if timeline == nil {
		err = fmt.Errorf("replay event timeline is required")
		return
	}
	if options.OutputPath == "" {
		err = fmt.Errorf("video output path is required")
		return
	}
	if options.FPS == 0 {
		options.FPS = 30
	}
	if options.Speed == 0 {
		options.Speed = 20
	}
	if options.CRF == 0 {
		options.CRF = 20
	}
	if options.Scale == 0 {
		options.Scale = 1
	}
	if options.FFmpegPath == "" {
		options.FFmpegPath = "ffmpeg"
	}
	if options.CRF < 16 || options.CRF > 35 {
		err = fmt.Errorf("video quality CRF must be between 16 and 35")
		return
	}

	var frame Frame
	if frame, options, err = prepareFrame(baseFrame, options); err != nil {
		return
	}
	if frame.Duration <= 0 {
		err = fmt.Errorf("replay has no positive battle duration")
		return
	}

	if _, err = videoFrameCount(frame.Duration, options.FPS, options.Speed, frame.Layout.Width, frame.Layout.Height); err != nil {
		return
	}

	var encoderPath string
	if encoderPath, err = exec.LookPath(options.FFmpegPath); err != nil {
		err = fmt.Errorf("find FFmpeg executable: %w", err)
		return
	}

	if err = os.MkdirAll(filepath.Dir(options.OutputPath), 0o755); err != nil {
		return
	}

	var reserved *os.File
	if reserved, err = os.OpenFile(options.OutputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
		return
	}

	if err = reserved.Close(); err != nil {
		os.Remove(options.OutputPath) // #nosec G104 -- cleanup must not replace the close error.
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
			os.Remove(options.OutputPath) // #nosec G104 -- best-effort cleanup; primary export error is returned.
		}
	}()

	var temporary *os.File
	if temporary, err = os.CreateTemp(filepath.Dir(options.OutputPath), ".wows-replay-*.mp4"); err != nil {
		return
	}

	temporaryPath = temporary.Name()
	if err = temporary.Close(); err != nil {
		return
	}

	var (
		stderr  bytes.Buffer
		rgba    *image.RGBA = image.NewRGBA(image.Rect(0, 0, frame.Layout.Width, frame.Layout.Height))
		command *exec.Cmd   = exec.CommandContext(ctx, encoderPath, // #nosec G204 -- executable is user-selected; fixed args are passed without a shell.
			"-hide_banner", "-loglevel", "error", "-nostats", "-y",
			"-f", "rawvideo", "-pixel_format", "rgba", "-video_size", fmt.Sprintf("%dx%d", frame.Layout.Width, frame.Layout.Height),
			"-framerate", strconv.Itoa(options.FPS), "-i", "pipe:0", "-an", "-c:v", "libx264", "-preset", "veryfast",
			"-crf", strconv.Itoa(options.CRF), "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-f", "mp4", temporaryPath,
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

	if err = RenderFrames(ctx, timeline, baseFrame, options, func(_ int, _ float64, renderedFrame image.Image) error {
		return writeRGBAFrame(input, renderedFrame, rgba)
	}); err != nil {
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

	if err = os.Rename(temporaryPath, options.OutputPath); err != nil {
		return
	}

	completed = true
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
	var framePixels []byte
	if rgba, ok := frame.(*image.RGBA); ok && rgba.Bounds().Min == (image.Point{}) && rgba.Stride == pixels.Stride && len(rgba.Pix) == len(pixels.Pix) {
		framePixels = rgba.Pix
	} else {
		draw.Draw(pixels, pixels.Bounds(), frame, frame.Bounds().Min, draw.Src)
		framePixels = pixels.Pix
	}

	var offset int
	for offset < len(framePixels) {
		var written int
		if written, err = writer.Write(framePixels[offset:]); err != nil {
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
