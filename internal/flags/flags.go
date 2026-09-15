package flags

// Ship and vehicle status flags (Flags field).
const (
	Docked                    uint32 = 1 << 0  // 0x00000001 - Docked on a landing pad
	Landed                    uint32 = 1 << 1  // 0x00000002 - Landed on planet surface
	LandingGearDown           uint32 = 1 << 2  // 0x00000004 - Landing gear deployed
	ShieldsUp                 uint32 = 1 << 3  // 0x00000008 - Shields up
	Supercruise               uint32 = 1 << 4  // 0x00000010 - In supercruise
	FlightAssistOff           uint32 = 1 << 5  // 0x00000020 - Flight assist off
	HardpointsDeployed        uint32 = 1 << 6  // 0x00000040 - Hardpoints deployed
	InWing                    uint32 = 1 << 7  // 0x00000080 - In wing / team
	LightsOn                  uint32 = 1 << 8  // 0x00000100 - Ship/vehicle lights on
	CargoScoopDeployed        uint32 = 1 << 9  // 0x00000200 - Cargo scoop deployed
	SilentRunning             uint32 = 1 << 10 // 0x00000400 - Silent running active
	ScoopingFuel              uint32 = 1 << 11 // 0x00000800 - Fuel scooping active
	SRVHandbrake              uint32 = 1 << 12 // 0x00001000 - SRV handbrake engaged
	SRVTurret                 uint32 = 1 << 13 // 0x00002000 - SRV using turret view
	SRVUnderShip              uint32 = 1 << 14 // 0x00004000 - SRV turret retracted (close to ship)
	SRVDriveAssist            uint32 = 1 << 15 // 0x00008000 - SRV drive assist on
	FSDMassLocked             uint32 = 1 << 16 // 0x00010000 - FSD mass locked
	FSDCharging               uint32 = 1 << 17 // 0x00020000 - FSD charging
	FSDCooldown               uint32 = 1 << 18 // 0x00040000 - FSD cooling down
	LowFuel                   uint32 = 1 << 19 // 0x00080000 - Low fuel (< 25%)
	Overheating               uint32 = 1 << 20 // 0x00100000 - Overheating (> 100%)
	HasLatLong                uint32 = 1 << 21 // 0x00200000 - Latitude/longitude available
	IsInDanger                uint32 = 1 << 22 // 0x00400000 - In danger
	BeingInterdicted          uint32 = 1 << 23 // 0x00800000 - Being interdicted
	InMainShip                uint32 = 1 << 24 // 0x01000000 - In main ship
	InFighter                 uint32 = 1 << 25 // 0x02000000 - In fighter
	InSRV                     uint32 = 1 << 26 // 0x04000000 - In SRV
	InAnalysisMode            uint32 = 1 << 27 // 0x08000000 - HUD in analysis mode
	NightVision               uint32 = 1 << 28 // 0x10000000 - Night vision enabled
	AltitudeFromAverageRadius uint32 = 1 << 29 // 0x20000000 - Altitude based on planet's average radius
	FSDJump                   uint32 = 1 << 30 // 0x40000000 - FSD jump underway
	SRVHighBeam               uint32 = 1 << 31 // 0x80000000 - SRV high beams on
)

// Odyssey on-foot status flags (Flags2 field).
const (
	OnFoot                uint32 = 1 << 0  // 0x00000001 - Player is on foot
	InTaxi                uint32 = 1 << 1  // 0x00000002 - In taxi / dropship / shuttle
	InMulticrew           uint32 = 1 << 2  // 0x00000004 - In multicrew (someone else's ship)
	OnFootInStation       uint32 = 1 << 3  // 0x00000008 - On foot in station
	OnFootOnPlanet        uint32 = 1 << 4  // 0x00000010 - On foot on planet
	AimDownSight          uint32 = 1 << 5  // 0x00000020 - Aiming down sight
	LowOxygen             uint32 = 1 << 6  // 0x00000040 - Low oxygen warning
	LowHealth             uint32 = 1 << 7  // 0x00000080 - Low health warning
	Cold                  uint32 = 1 << 8  // 0x00000100 - Cold temperature state
	Hot                   uint32 = 1 << 9  // 0x00000200 - Hot temperature state
	VeryCold              uint32 = 1 << 10 // 0x00000400 - Very cold temperature state
	VeryHot               uint32 = 1 << 11 // 0x00000800 - Very hot temperature state
	GlideMode             uint32 = 1 << 12 // 0x00001000 - In glide mode
	OnFootInHangar        uint32 = 1 << 13 // 0x00002000 - On foot in hangar
	OnFootSocialSpace     uint32 = 1 << 14 // 0x00004000 - On foot in social space
	OnFootExterior        uint32 = 1 << 15 // 0x00008000 - On foot exterior
	BreathableAtmosphere  uint32 = 1 << 16 // 0x00010000 - In breathable atmosphere
	TelepresenceMulticrew uint32 = 1 << 17 // 0x00020000 - In telepresence multicrew
	PhysicalMulticrew     uint32 = 1 << 18 // 0x00040000 - In physical multicrew
	FSDHyperdriveCharging uint32 = 1 << 19 // 0x00080000 - FSD hyperdrive charging
)

// GuiFocus menu constants.
const (
	GuiFocusNone            uint32 = 0
	GuiFocusInternalPanel   uint32 = 1  // Right hand side (Internal panel)
	GuiFocusExternalPanel   uint32 = 2  // Left hand side (External panel / Navigation)
	GuiFocusCommsPanel      uint32 = 3  // Top (Communications panel)
	GuiFocusRolePanel       uint32 = 4  // Bottom (Role panel)
	GuiFocusStationServices uint32 = 5  // Station services menu
	GuiFocusGalaxyMap       uint32 = 6  // Galaxy map
	GuiFocusSystemMap       uint32 = 7  // System map
	GuiFocusOrrery          uint32 = 8  // Orrery view
	GuiFocusFSSMode         uint32 = 9  // Full Spectrum Scanner (FSS) mode
	GuiFocusSAAMode         uint32 = 10 // Surface Access Analysis (Detailed Surface Scanner) mode
	GuiFocusCodex           uint32 = 11 // Codex view

	// Helper aliases
	GuiFocusLeft   uint32 = GuiFocusExternalPanel
	GuiFocusRight  uint32 = GuiFocusInternalPanel
	GuiFocusTop    uint32 = GuiFocusCommsPanel
	GuiFocusBottom uint32 = GuiFocusRolePanel
)

// GuiFocusString returns a human-readable name for a GuiFocus value.
func GuiFocusString(focus uint32) string {
	switch focus {
	case GuiFocusNone:
		return "No Focus"
	case GuiFocusInternalPanel:
		return "Internal Panel (Right)"
	case GuiFocusExternalPanel:
		return "External Panel (Left)"
	case GuiFocusCommsPanel:
		return "Comms Panel (Top)"
	case GuiFocusRolePanel:
		return "Role Panel (Bottom)"
	case GuiFocusStationServices:
		return "Station Services"
	case GuiFocusGalaxyMap:
		return "Galaxy Map"
	case GuiFocusSystemMap:
		return "System Map"
	case GuiFocusOrrery:
		return "Orrery"
	case GuiFocusFSSMode:
		return "FSS Mode"
	case GuiFocusSAAMode:
		return "SAA Mode"
	case GuiFocusCodex:
		return "Codex"
	default:
		return "Unknown"
	}
}
