package roundtable

import "github.com/scottzx/1Agents/backend/internal/appregistry"

// AppID is the stable manifest namespace (design §6.3).
const AppID = "agents-roundtable"

// MountL1ID is the l1-page mount id used by the frontend enterL1App path.
const MountL1ID = "agents-roundtable"

// ViewAgentsRoundtable is the frontend registerAppView key.
const ViewAgentsRoundtable = "AgentsRoundtable"

func init() {
	// Discovery / 应用中心 / 更多应用 entry (design §6.3).
	// Enabled by default so users need no manual config to launch.
	appregistry.Register(appregistry.AppManifest{
		ID:      AppID,
		Name:    "圆桌讨论",
		Version: "0.1.0",
		Enabled: true,
		MountPoints: []appregistry.MountPoint{
			{
				Type:  "l1-page",
				ID:    MountL1ID,
				Label: "圆桌讨论",
				View:  ViewAgentsRoundtable,
				Icon:  "users",
			},
		},
		TaskTypes:    []string{},
		DomainTables: []string{},
	})
}
