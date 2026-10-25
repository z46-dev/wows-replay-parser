package video

import (
	"bytes"
	"context"
	"image"
	"testing"

	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
)

// TestWriteRGBAFrameWritesRenderedRGBAWithoutChangingPixels covers the direct encoder path.
func TestWriteRGBAFrameWritesRenderedRGBAWithoutChangingPixels(t *testing.T) {
	var (
		frame  *image.RGBA = image.NewRGBA(image.Rect(0, 0, 3, 2))
		pixels *image.RGBA = image.NewRGBA(frame.Bounds())
		output bytes.Buffer
	)
	for index := range frame.Pix {
		frame.Pix[index] = byte(index * 7)
	}

	if err := writeRGBAFrame(&output, frame, pixels); err != nil {
		t.Fatalf("writeRGBAFrame() error = %v", err)
	}
	if !bytes.Equal(output.Bytes(), frame.Pix) {
		t.Fatalf("written bytes = %v, want %v", output.Bytes(), frame.Pix)
	}
}

// TestRenderFramesPreservesOrderWithConcurrentWorkers verifies callback ordering and clocks.
func TestRenderFramesPreservesOrderWithConcurrentWorkers(t *testing.T) {
	var (
		frame Frame = Frame{
			Map:         image.NewRGBA(image.Rect(0, 0, 2, 2)),
			Duration:    0.1,
			WorldBounds: Bounds{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1},
		}
		indices []int
		clocks  []float64
	)
	if err := RenderFrames(context.Background(), &Timeline{}, frame, Options{FPS: 1, Speed: 20, Scale: 0.75, Workers: 2}, func(index int, clock float64, _ image.Image) error {
		indices = append(indices, index)
		clocks = append(clocks, clock)
		return nil
	}); err != nil {
		t.Fatalf("RenderFrames() error = %v", err)
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 || clocks[0] != 0 || clocks[1] != 0.1 {
		t.Fatalf("frames = %v at clocks %v, want [0 1] at [0 0.1]", indices, clocks)
	}
}

// TestVideoRenderWorkerCountUsesAutomaticOrExplicitCounts verifies worker selection without an arbitrary cap.
func TestVideoRenderWorkerCountUsesAutomaticOrExplicitCounts(t *testing.T) {
	for _, workers := range []int{0, 1, 4, 8, 128} {
		var count int
		var err error
		if count, err = renderWorkerCount(workers); err != nil {
			t.Fatalf("renderWorkerCount(%d) error = %v", workers, err)
		}
		if count < 1 || workers != 0 && count != workers {
			t.Fatalf("renderWorkerCount(%d) = %d", workers, count)
		}
	}
	if _, err := renderWorkerCount(-1); err == nil {
		t.Fatal("renderWorkerCount(-1) should reject a negative pool")
	}
}

// TestPrepareFrameScalesTheCompleteLayout verifies a single quality control scales all layout regions.
func TestPrepareFrameScalesTheCompleteLayout(t *testing.T) {
	var (
		baseFrame Frame = Frame{Layout: Layout{Width: 1328, Height: 800, LeftPanelWidth: 280, RightPanelWidth: 280, TopBarHeight: 40}}
		frame     Frame
		options   Options
		err       error
	)
	if frame, options, err = prepareFrame(baseFrame, Options{Scale: 1.5}); err != nil {
		t.Fatalf("prepareFrame() error = %v", err)
	}
	if frame.Layout.Width != 1992 || frame.Layout.Height != 1200 || frame.Layout.LeftPanelWidth != 420 || frame.Layout.RightPanelWidth != 420 || frame.Layout.TopBarHeight != 60 {
		t.Fatalf("scaled layout = %+v, want 1992x1200 with 420px panels and 60px top bar", frame.Layout)
	}
	if options.FPS != 30 || options.Speed != 20 || options.Scale != 1.5 {
		t.Fatalf("normalized options = %+v", options)
	}
}

// TestFrameStateUsesVictoryPointEvents checks authoritative score updates and their current rate.
func TestFrameStateUsesVictoryPointEvents(t *testing.T) {
	var (
		state *FrameState = NewFrameState(&Timeline{Events: []events.Event{
			{Clock: 100, Type: events.VictoryPointsEvent, VictoryPoints: &events.VictoryPointUpdate{TotalPoints: 1500}},
			{Clock: 104, Type: events.VictoryPointsEvent, VictoryPoints: &events.VictoryPointUpdate{TotalPoints: 1800}},
		}})
		frame Frame = state.FrameAt(Frame{LeftStartingPoints: 300, RightStartingPoints: 300}, 104)
	)
	if frame.LeftTeamPoints != 318 || frame.RightTeamPoints != 300 || frame.LeftPointsPerSec != 0.75 {
		t.Fatalf("score frame = %.1f/%.1f points at %.2f/%.2f points per second", frame.LeftTeamPoints, frame.RightTeamPoints, frame.LeftPointsPerSec, frame.RightPointsPerSec)
	}
}

func TestFrameAtPreservesResolvedSquadronIcon(t *testing.T) {
	var timeline Timeline = Timeline{Events: []events.Event{{
		Clock: 10,
		Type:  events.MinimapSquadronEvent,
		Squadron: &events.MinimapSquadron{
			PlaneID:      7,
			GameParamsID: 42,
			X:            5,
			Z:            8,
		},
	}}}
	var state *FrameState = NewFrameState(&timeline)
	var frame Frame = state.FrameAt(Frame{PlaneIconNames: map[uint32]string{42: "controllable/torpedo_regular"}}, 10)
	if len(frame.Squadrons) != 1 || frame.Squadrons[0].IconName != "controllable/torpedo_regular" {
		t.Fatalf("squadrons = %+v, want the resolved aircraft icon", frame.Squadrons)
	}
}

func TestFrameAtShowsKillsFromLiveDeathEvents(t *testing.T) {
	var state *FrameState = NewFrameState(&Timeline{Events: []events.Event{
		{Clock: 10, Type: events.ShipDamageDealtEvent, DamageDealt: &events.ShipDamageDealt{VictimID: 22, AggressorID: 11, Damage: 12474}},
		{Clock: 20, Type: events.ShipDeathEvent, Death: &events.ShipDeath{VictimID: 22, KillerID: 11}},
	}})
	state.ships[11] = render.Ship{EntityID: 11, TeamID: 1, TeamKnown: true, MaxHealth: 100, Health: 100}
	state.ships[22] = render.Ship{EntityID: 22, TeamID: 2, TeamKnown: true, MaxHealth: 200, Health: 200}

	var frame Frame = state.FrameAt(Frame{LeftTeam: []render.Player{{Name: "killer", TeamID: 1, MaxHealth: 100}}}, 20)
	if frame.LeftTeam[0].Kills != 1 || frame.LeftTeam[0].Damage != 12474 || frame.LeftTeam[0].EntityID != 11 {
		t.Fatalf("live combat row = %+v, want entity 11 with 12474 damage and one kill", frame.LeftTeam[0])
	}
}

func TestBindShipLineupUsesPerspectiveAppropriateDetection(t *testing.T) {
	var (
		ships []render.Ship = []render.Ship{
			{EntityID: 11, TeamID: 1, TeamKnown: true, MaxHealth: 100, DetectedByEnemy: true},
			{EntityID: 22, TeamID: 2, TeamKnown: true, MaxHealth: 200, Enemy: true, Spotted: true},
		}
		allies  []render.Player = []render.Player{{TeamID: 1, MaxHealth: 100}}
		enemies []render.Player = []render.Player{{TeamID: 2, MaxHealth: 200}}
	)
	BindShipLineup(ships, allies)
	BindShipLineup(ships, enemies)
	if !allies[0].Detected || !enemies[0].Detected {
		t.Fatalf("detected states = ally %v, enemy %v; want both perspective-correct", allies[0].Detected, enemies[0].Detected)
	}
	ships[0].DetectedByEnemy = false
	allies = []render.Player{{TeamID: 1, MaxHealth: 100}}
	BindShipLineup(ships, allies)
	if allies[0].Detected {
		t.Fatal("ally should not be marked detected when visibilityFlags is zero")
	}
}

func TestFrameAtRevealsResultStatsOnlyAtResultClock(t *testing.T) {
	var timeline Timeline = Timeline{Events: []events.Event{{
		Clock: 10,
		Type:  events.BattleResultsEvent,
		Results: &events.BattleResults{Summary: &events.BattleSummary{Players: map[uint64]events.BattlePlayerResult{
			7: {AccountDBID: 7, Damage: 42000, ShipsKilled: 2},
		}}},
	}}}
	var state *FrameState = NewFrameState(&timeline)
	var base Frame = Frame{LeftTeam: []Player{{AccountDBID: 7}}}
	var before Frame = state.FrameAt(base, 9)
	if before.LeftTeam[0].Damage != 0 || before.LeftTeam[0].Kills != 0 {
		t.Fatalf("pre-result stats = %.0f damage / %.0f kills, want zero", before.LeftTeam[0].Damage, before.LeftTeam[0].Kills)
	}
	var after Frame = state.FrameAt(base, 10)
	if after.LeftTeam[0].Damage != 42000 || after.LeftTeam[0].Kills != 2 {
		t.Fatalf("result stats = %.0f damage / %.0f kills, want 42000 / 2", after.LeftTeam[0].Damage, after.LeftTeam[0].Kills)
	}
}

func TestFrameStateUsesBothBattleLogicTeamScores(t *testing.T) {
	var timeline events.Timeline = events.Timeline{Events: []events.Event{
		{Clock: 10, Type: events.TeamScoreEvent, TeamScore: &events.TeamScoreUpdate{TeamIndex: 0, Score: 640}},
		{Clock: 10, Type: events.TeamScoreEvent, TeamScore: &events.TeamScoreUpdate{TeamIndex: 1, Score: 713}},
	}}
	var state *FrameState = NewFrameState(&timeline)
	state.ownTeamID = 1
	state.ownTeamKnown = true
	var frame Frame = state.FrameAt(Frame{LeftStartingPoints: 300, RightStartingPoints: 300}, 10)
	if frame.LeftTeamPoints != 713 || frame.RightTeamPoints != 640 {
		t.Fatalf("team score display = (%.0f, %.0f), want (713, 640)", frame.LeftTeamPoints, frame.RightTeamPoints)
	}
}

// TestReplayDurationIncludesEventsAfterBattleResults protects battle-end animation frames.
func TestReplayDurationIncludesEventsAfterBattleResults(t *testing.T) {
	var timeline Timeline = Timeline{Events: []events.Event{{Clock: 557.7142}}}
	if duration := replayDuration(503, &timeline, 1200); duration < 557.71 || duration > 557.72 {
		t.Fatalf("replayDuration() = %.4f, want final event at 557.7142", duration)
	}
}
