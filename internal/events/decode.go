package events

import (
	"fmt"
	"github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
)

// Decode converts supported packet payloads into a timeline without discarding unknown packets.
func Decode(replay *replayparser.Replay) (timeline *Timeline, err error) {
	return DecodeWithSchema(replay, nil)
}

// DecodeWithSchema decodes replay events and resolves entity member indices when schemas are supplied.
func DecodeWithSchema(replay *replayparser.Replay, schema *replaydata.Schema) (timeline *Timeline, err error) {
	timeline = &Timeline{
		Events:         make([]Event, 0),
		UnknownPackets: make(map[uint32]int),
	}

	var (
		entityTypes map[uint32]uint16  = make(map[uint32]uint16)
		shipHealth  map[uint32]float32 = make(map[uint32]float32)
	)

	for index, packet := range replay.Packets {
		var event Event

		switch packet.Type {
		case basePlayerCreatePacketType:
			if event, err = decodeBasePlayerCreate(packet, BasePlayerCreateEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			entityTypes[event.EntityID] = event.Bootstrap.EntityTypeID
			timeline.Events = append(timeline.Events, event)
		case basePlayerCreateStubType:
			if event, err = decodeBasePlayerCreate(packet, BasePlayerCreateStubEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			entityTypes[event.EntityID] = event.Bootstrap.EntityTypeID
			timeline.Events = append(timeline.Events, event)
		case cellPlayerCreatePacketType:
			if event, err = decodeCellPlayerCreate(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityEnterPacketType:
			if event, err = decodeEntityEnter(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case serverTickPacketType:
			if event, err = decodeFloat64(packet, ServerTickEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case serverTimestampPacketType:
			if event, err = decodeFloat64(packet, ServerTimestampEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case initFlagPacketType:
			if event, err = decodeInitFlag(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case initMarkerPacketType:
			if event, err = decodeInitMarker(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case versionPacketType:
			if event, err = decodeVersion(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case gunMarkerPacketType:
			if event, err = decodeGunMarker(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case playerNetStatsPacketType:
			if event, err = decodePlayerNetStats(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityLeavePacketType:
			if event, err = decodeEntityLeave(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityCreatePacketType:
			if event, err = decodeEntityCreate(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			entityTypes[event.EntityID] = event.Created.EntityTypeID
			if schema != nil {
				event.Created.EntityType = schema.EntityName(event.Created.EntityTypeID)
			}

			timeline.Events = append(timeline.Events, event)
		case entityPropertyPacketType:
			if event, err = decodeEntityMember(packet, EntityPropertyEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
			var entityTypeID uint16 = entityTypes[event.EntityID]
			if schema != nil {
				event.Member.EntityType = schema.EntityName(entityTypeID)
				event.Member.Name = schema.PropertyName(entityTypeID, event.Member.MemberID)
			}

			if event.Member.Name == "health" && event.Member.PayloadSize == 4 {
				var healthEvent Event
				if healthEvent, err = decodeShipHealth(packet, event.EntityID, shipHealth); err != nil {
					err = fmt.Errorf("packet %d: %w", index, err)
					return
				}

				timeline.Events = append(timeline.Events, healthEvent)
				if healthEvent.Health.Delta < 0 {
					healthEvent.Type = ShipDamageEvent
					timeline.Events = append(timeline.Events, healthEvent)
				} else if healthEvent.Health.Delta > 0 {
					healthEvent.Type = ShipRepairEvent
					timeline.Events = append(timeline.Events, healthEvent)
				}
			}
		case entityMethodPacketType:
			if event, err = decodeEntityMember(packet, EntityMethodEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			if schema != nil {
				var entityTypeID uint16 = entityTypes[event.EntityID]
				event.Member.EntityType = schema.EntityName(entityTypeID)
				event.Member.Name = schema.MethodName(entityTypeID, event.Member.MemberID)
			}

			timeline.Events = append(timeline.Events, event)
		case entityControlPacketType:
			if event, err = decodeEntityControl(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case positionPacketType:
			if event, err = decodePosition(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case ownedShipPacketType:
			if event, err = decodeOwnShip(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.OwnShipID = event.EntityID
			timeline.Events = append(timeline.Events, event)
		case playerOrientationType:
			if event, err = decodePlayerOrientation(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case battleResultsPacketType:
			if event, err = decodeBattleResults(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			if schema != nil {
				for accountDBID, player := range event.Results.Summary.Players {
					if vehicle, exists := schema.Vehicle(player.VehicleTypeID); exists {
						player.ShipID = vehicle.ShipID
						player.ShipName = vehicle.ShipName
						event.Results.Summary.Players[accountDBID] = player
					}
				}
			}

			timeline.Events = append(timeline.Events, event)
		case nestedPropertyPacketType:
			if event, err = decodeNestedProperty(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case nonVolatilePacketType:
			if event, err = decodeNonVolatilePosition(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cameraPacketType:
			if event, err = decodeCamera(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cameraModePacketType:
			if event, err = decodeCameraMode(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case mapPacketType:
			if event, err = decodeMap(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case legacyControllerPacketType:
			if event, err = decodeLegacyController(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cameraFreeLookPacketType:
			if event, err = decodeCameraFreeLook(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case weaponLockPacketType:
			if event, err = decodeWeaponLock(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case subControllerPacketType:
			if event, err = decodeSubController(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cruiseStatePacketType:
			if event, err = decodeCruiseState(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case shotTrackingPacketType:
			if event, err = decodeShotTracking(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case replayMarkerPacketType:
			event = Event{Clock: packet.Clock, Type: ReplayMarkerEvent, Payload: packet.Payload}
			timeline.Events = append(timeline.Events, event)
		default:
			timeline.UnknownPackets[packet.Type]++
		}
	}

	return
}

// Filter returns timeline events that meet every supplied filter.
func Filter(events []Event, eventType Type, entityID uint32) (filtered []Event) {
	filtered = make([]Event, 0, len(events))

	for _, event := range events {
		if eventType != "" && event.Type != eventType {
			continue
		}

		if entityID != 0 && event.EntityID != entityID {
			continue
		}

		filtered = append(filtered, event)
	}

	return
}
