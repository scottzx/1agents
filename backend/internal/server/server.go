package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/appkit"
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/auth"
	"github.com/scottzx/1Agents/backend/internal/ccconnect"
	"github.com/scottzx/1Agents/backend/internal/config"
	"github.com/scottzx/1Agents/backend/internal/contacts"
	ctxt "github.com/scottzx/1Agents/backend/internal/context"
	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/digest"
	"github.com/scottzx/1Agents/backend/internal/domainownership"
	"github.com/scottzx/1Agents/backend/internal/fs"
	"github.com/scottzx/1Agents/backend/internal/gateway"
	"github.com/scottzx/1Agents/backend/internal/git"
	"github.com/scottzx/1Agents/backend/internal/harnesskit"
	"github.com/scottzx/1Agents/backend/internal/ingest"
	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/roundtable"
	"github.com/scottzx/1Agents/backend/internal/sources"
	"github.com/scottzx/1Agents/backend/internal/supervisor"
	"github.com/scottzx/1Agents/backend/internal/system"
	"github.com/scottzx/1Agents/backend/internal/taskapi"
	"github.com/scottzx/1Agents/backend/internal/templateregistry"
	"github.com/scottzx/1Agents/backend/internal/terminal"
	"github.com/scottzx/1Agents/backend/internal/tunnel"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/api/workspace/*  → Workspace CRUD handlers (Go, JSON file storage)
//	/api/agent/*      → 1agents-side chat session index (Go, JSON file storage)
//	/api/terminal/*   → Tmux terminal session management (create/list/kill/switch)
//	/api/system/*     → System management: version info, OTA self-update
//	/ws               → Reverse-proxy to ttyd WebSocket endpoint
//	/token            → Reverse-proxy to ttyd auth-token endpoint
//	/                 → Static file server (compiled frontend assets)
func NewRouter(cfg *config.Config, harnessKitRuntime ...harnesskit.Runtime) http.Handler {
	mux := http.NewServeMux()

	// ── File system API ──────────────────────────────────────────────────────
	fsHandler := fs.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/fs/list", fsHandler.List)              // GET  ?path=.
	mux.HandleFunc("/api/fs/search", fsHandler.Search)          // GET  ?query=xxx&tag=all/doc/img/code
	mux.HandleFunc("/api/fs/read", fsHandler.Read)              // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/view", fsHandler.View)              // GET  ?path=./page.html (serves with correct content-type)
	mux.HandleFunc("/api/fs/view/", fsHandler.View)             // GET  /api/fs/view/relative/path (prefix route for relative assets support)
	mux.HandleFunc("/api/fs/image", fsHandler.Image)            // GET  ?path=./image.png (returns base64 data URL, deprecated)
	mux.HandleFunc("/api/fs/image/", fsHandler.ImageStream)     // GET  /api/fs/image/relative/path (streams raw bytes; preferred)
	mux.HandleFunc("/api/fs/write", fsHandler.Write)            // POST ?path=./main.go
	mux.HandleFunc("/api/fs/upload", fsHandler.Upload)          // POST multipart/form-data (field "file") → saves to /tmp, returns {path,name}
	mux.HandleFunc("/api/fs/upload-to", fsHandler.UploadTo)     // POST multipart/form-data (field "file") → saves to specified path
	mux.HandleFunc("/api/fs/open-folder", fsHandler.OpenFolder) // POST ?path=... → opens folder in Finder/Explorer
	mux.HandleFunc("/api/fs/rename", fsHandler.Rename)          // POST ?oldPath=...&newPath=...
	mux.HandleFunc("/api/fs/copy", fsHandler.Copy)              // POST ?src=...&dst=... (file or directory tree)
	mux.HandleFunc("/api/fs/mkdir", fsHandler.Mkdir)            // POST ?path=./newdir
	mux.HandleFunc("/api/fs/delete", fsHandler.Delete)          // DELETE ?path=./main.go

	// ── Workspace API ────────────────────────────────────────────────────────
	wsHandler := workspace.NewHandler(cfg.TmuxSession)
	if len(harnessKitRuntime) > 0 && harnessKitRuntime[0] != nil {
		wsHandler.SetHarnessKitRuntime(harnessKitRuntime[0])
	}
	if err := wsHandler.EnsureDefaultWorkspace(); err != nil {
		log.Printf("[server] ensure default workspace: %v", err)
	}
	mux.HandleFunc("/api/workspace/list", wsHandler.List)                                 // GET
	mux.HandleFunc("/api/workspace/create", wsHandler.Create)                             // POST
	mux.HandleFunc("/api/workspace/update", wsHandler.Update)                             // POST
	mux.HandleFunc("/api/workspace/skills", wsHandler.WorkspaceSkills)                    // GET ?id= — synced skills + drift status
	mux.HandleFunc("/api/workspace/push-skill", wsHandler.PushSkill)                      // POST {id, skillRef} — push edited copy back to 母体
	mux.HandleFunc("/api/workspace/pull-skill", wsHandler.PullSkill)                      // POST {id, skillRef} — pull latest from 母体 into workspace copy
	mux.HandleFunc("/api/workspace/available-skills", wsHandler.AvailableSkills)          // GET ?id= — 母体 skills the project can add
	mux.HandleFunc("/api/workspace/add-skill", wsHandler.AddSkill)                        // POST {id, skillRef} — materialize a 母体 skill into the project
	mux.HandleFunc("/api/workspace/remove-skill", wsHandler.RemoveSkill)                  // POST {id, skillRef} — delete a skill from the project only
	mux.HandleFunc("/api/workspace/agents", wsHandler.WorkspaceAgents)                    // GET ?id= — synced agents + drift status
	mux.HandleFunc("/api/workspace/push-agent", wsHandler.PushAgent)                      // POST {id, agentRef} — push edited copy back to 母体
	mux.HandleFunc("/api/workspace/soul", wsHandler.WorkspaceSoul)                        // GET ?id= / POST {id, content} — assistant persona SOUL.md
	mux.HandleFunc("/api/workspace/team", wsHandler.WorkspaceTeam)                        // GET ?id= / POST {id, primary} — agent-team manifest (primary + members)
	mux.HandleFunc("/api/workspace/available-agents", wsHandler.WorkspaceAvailableAgents) // GET ?id= — 母体 agents the project can add
	mux.HandleFunc("/api/workspace/add-agent", wsHandler.AddAgent)                        // POST {id, agentRef} — materialize a 母体 agent into the project
	mux.HandleFunc("/api/workspace/remove-agent", wsHandler.RemoveAgent)                  // POST {id, agentRef} — delete an agent from the project only
	mux.HandleFunc("/api/assistant/souls", wsHandler.ListSouls)                           // GET ?lang= — curated persona presets
	mux.HandleFunc("/api/workspace/reorder", wsHandler.Reorder)                           // POST
	mux.HandleFunc("/api/workspace/delete", wsHandler.Delete)                             // DELETE ?id=xxx
	mux.HandleFunc("/api/workspace/pick-directory", wsHandler.PickDirectory)              // POST — opens native folder picker
	mux.HandleFunc("/api/workspace/list-directories", wsHandler.ListDirectories)          // GET ?path=...
	mux.HandleFunc("/api/workspace/create-directory", wsHandler.CreateDirectory)          // POST
	mux.HandleFunc("/api/workspace/upload-avatar", wsHandler.UploadAvatar)                // POST multipart file → {url}
	mux.Handle("/avatars/", workspace.ServeAvatars())                                     // GET avatar files (embedded presets + uploads)

	// ── App registry API (Wave 2a, #330) ────────────────────────────────────
	appregistry.StartupDiagnostics()                     // C0 capability check (design §6.3)
	mux.HandleFunc("/api/apps", appregistry.HandleList)  // GET → {apps:[...]}
	mux.HandleFunc("/api/apps/", appregistryItemHandler) // POST /{id}/enable|disable

	// ── LLM Provider API (ClawBox / ~/.1agents/providers.json) ─────────────
	mux.HandleFunc("/api/providers", handleProviders)                              // GET, POST, DELETE
	mux.HandleFunc("/api/providers/switch", handleProviderSwitch)                  // POST {id}
	mux.HandleFunc("/api/providers/fetch-models", handleFetchModels)               // POST {base_url, api_key}
	mux.HandleFunc("/api/providers/discover-models", handleDiscoverProviderModels) // POST {provider_id}
	mux.HandleFunc("/api/provider-models", handleProviderModels)                   // GET ?provider_id=
	mux.HandleFunc("/api/agents/switch", handleAgentSwitch)                        // POST {agent, provider_id, model, ...}
	mux.HandleFunc("/api/agents/runtime", handleAgentRuntime)                      // GET
	mux.HandleFunc("/api/agents/options", handleAgentOptions)                      // GET
	mux.HandleFunc("/api/agents/binding", handleAgentBinding)                      // POST {binding, apply}

	// ── Domain ownership gate (C0, design §7/§13.3) ────────────────────────
	// Registers the kernel ownership ledger, ensures the denial-audit table
	// and installs the persistent sink so cross-domain access denials are
	// audited. Query-path permission denials are audited via the hook.
	if db, err := meta.OpenDefault(); err == nil {
		if err := domainownership.StartupGate(db.SQL()); err != nil {
			log.Printf("[server] domain ownership gate: %v", err)
		}
	}
	domainownership.WireQueryDenialAudit()

	// ── Product Shell Registry API (C0, design §8/D7) ──────────────────────
	mux.HandleFunc("/api/shells", appregistry.HandleShellList) // GET → {shells, defaultShell, userPreference, effectiveDefault}
	mux.HandleFunc("/api/shells/", shellRegistryItemHandler)   // POST /{id}/enable|disable · PUT default|preference · GET composition

	// ── Template registry API (Wave 2a, #329) ───────────────────────────────
	mux.HandleFunc("/api/templates", templateregistry.HandleList) // GET → {templates:[...]}

	// ── Project config API (Wave 2a, #327) ──────────────────────────────────
	mux.HandleFunc("/api/project/config", projectConfigHandler) // GET ?id=, PUT

	// ── Agent (chat session) index API ─────────────────────────────────────
	// 1agents-side metadata store. The actual conversation lives in
	// cc-connect; these endpoints only index a session created via the
	// cc-connect REST/WS so the sidebar can list it like a terminal session.
	agentStore, err := agent.NewStore()
	if err != nil {
		log.Printf("[server] agent store init failed: %v", err)
	} else {
		// One-time import of the legacy JSON stores into ~/.1agents/meta.db
		// (renames the source files to *.migrated; no-op afterwards).
		if db, dbErr := meta.OpenDefault(); dbErr == nil {
			// Unify the legacy workspaces_dir.json into the projects table first
			// (renames the json to *.migrated), then import the remaining legacy
			// stores (sessions + per-workspace tasks.json), which read the workspace
			// paths back from the projects table.
			if migErr := db.ImportLegacyWorkspaces(); migErr != nil {
				log.Printf("[server] workspace registry migration: %v", migErr)
			}
			if migErr := db.MigrateLegacy(); migErr != nil {
				log.Printf("[server] legacy metadata migration: %v", migErr)
			}
			mux.HandleFunc("/api/projects", meta.ProjectsHandler(db))       // GET, POST
			mux.HandleFunc("/api/search", meta.SearchHandler(db))           // GET ?q=xxx — 对话历史 quick search over tasks + sessions
			mux.HandleFunc("/api/projects/", meta.ProjectActionHandler(db)) // POST {id}/archive|close|reopen

			// Workspace Inbox (#202 / #60): list + manual capture. Item actions
			// (archive/read/accept) and deliver register with PMO below so Accept
			// can share the TaskStore write lock.
			mux.HandleFunc("/api/inbox", meta.InboxHandler(meta.NewInboxStore(db))) // GET ?workspaceId=, POST capture
		}

		tasksStore, tsErr := agent.NewTasksStore()
		if tsErr != nil {
			log.Printf("[server] tasks store init failed: %v", tsErr)
		} else {
			// Share this store instance with the cc-connect IM notifier (#129)
			// so its approve/reject write-backs serialize with the scheduler's
			// per-workspace Mutate lock.
			ccconnect.SetTasksStore(tasksStore)

			acpxPort := acpxBridgePort()
			var turnStore *meta.AgentTurnStore
			if db, dbErr := meta.OpenDefault(); dbErr != nil {
				log.Printf("[server] Turn store init failed: %v", dbErr)
			} else {
				turnStore = meta.NewAgentTurnStore(db)
			}
			acpxClient := agent.NewAcpxClient(acpxPort, turnStore)

			scheduler := agent.NewScheduler(tasksStore, func() ([]agent.WorkspaceRef, error) {
				wsHandler := workspace.NewHandler()
				wsCfg, err := wsHandler.LoadWorkspacesConfig()
				if err != nil {
					return nil, err
				}
				refs := make([]agent.WorkspaceRef, len(wsCfg.Workspaces))
				for i, ws := range wsCfg.Workspaces {
					refs[i] = agent.WorkspaceRef{ID: ws.ID, Name: ws.Name, Path: ws.Path}
				}
				return refs, nil
			})
			// Loopback base for the PM/verifier task-tool MCP subprocess to call
			// back into this daemon's HTTP API (always http on 127.0.0.1; the
			// internal-token bypass in authMiddleware accepts it). The verifier
			// runner posts verdicts here via submit_review, so the runner needs
			// it too — built before the runner is wired.
			_, selfPort, _ := net.SplitHostPort(cfg.ListenAddr)
			selfBaseURL := "http://127.0.0.1:" + selfPort

			// Headless executor/verifier: scheduler-triggered tasks run through
			// the 1acp bridge with no frontend involved (automation-first).
			scheduler.SetRunner(agent.NewTaskRunner(acpxPort, selfBaseURL, tasksStore, agentStore, scheduler))

			// ── North Task API + executor=function dispatch (Epic #317 内核).
			// tasksStore is *meta.TaskStore (agent.TasksStore alias) and
			// agent.Task == meta.Task, so the bridge is direct. RunInits runs any
			// installable app's startup hook against the live API — no apps are
			// compiled in yet, so it is a no-op until one is registered.
			taskAPI := taskapi.New(tasksStore)
			scheduler.SetFunctionRunner(func(task agent.Task, wsPath string) {
				taskapi.RunFunction(task, wsPath, tasksStore, taskAPI)
			})
			appkit.RunInits(taskAPI)

			scheduler.Start(context.Background())

			// Probe installed agent CLIs once at startup; cached behind an
			// RWMutex and re-probable via /api/agent/catalog?refresh=1. Shared
			// process-wide with the cc-connect runner (which curates the
			// management API's creatable-agent list from the same probe).
			catalogStore := agent.DefaultCatalog()

			agentHandler := agent.NewHandler(agentStore, tasksStore, acpxClient, scheduler, catalogStore, selfBaseURL)
			// Deliver Outbox Events appended by the command gateway (#324):
			// at-least-once with retry/backoff and receipt dedup.
			agentHandler.StartOutboxDispatcher(5 * time.Second)
			mux.HandleFunc("/api/agent/agent-types", agentHandler.HandleAgentTypes)               // GET
			mux.HandleFunc("/api/agent/catalog", agentHandler.HandleAgentCatalog)                 // GET (?refresh=1)
			mux.HandleFunc("/api/agent/sessions", agentHandler.HandleSessionsRoot)                // GET, POST
			mux.HandleFunc("/api/agent/sessions/", agentHandler.HandleSessionsItem)               // GET, DELETE /{id}
			mux.HandleFunc("/api/agent/turns", agentHandler.HandleTurns)                          // GET cursor-paged Turn history
			mux.HandleFunc("/api/agent/activity", agentHandler.HandleProjectActivity)             // GET aggregated Project activity
			mux.HandleFunc("/api/agent/project-items", agentHandler.HandleTasksRoot)              // GET, POST
			mux.HandleFunc("/api/agent/project-items/resolve", agentHandler.HandleTaskResolve)    // GET ?project=&number= (more specific than the subtree below)
			mux.HandleFunc("/api/agent/project-items/", agentHandler.HandleTasksItem)             // DELETE /{id}
			mux.HandleFunc("/api/agent/work-cases", agentHandler.HandleWorkCasesRoot)             // GET, POST (#322)
			mux.HandleFunc("/api/agent/work-cases/", agentHandler.HandleWorkCasesItem)            // GET/PATCH/DELETE /{id}, transition, links, tasks, runs, events (#322)
			mux.HandleFunc("/api/agent/agenda", agentHandler.HandleAgendaRoot)                    // GET (cross-workspace agenda, #192)
			mux.HandleFunc("/api/agent/personal/aggregate", agentHandler.HandlePersonalAggregate) // GET Personal Shell cross-shell work aggregate (#329)
			mux.HandleFunc("/api/agent/milestones", agentHandler.HandleMilestonesRoot)            // GET, POST
			mux.HandleFunc("/api/agent/milestones/", agentHandler.HandleMilestonesItem)           // PATCH, DELETE /{id}, POST /reorder
			mux.HandleFunc("/api/agent/feature-catalog", agentHandler.HandleFeatureCatalogRoot)   // GET, POST
			mux.HandleFunc("/api/agent/feature-catalog/", agentHandler.HandleFeatureCatalogItem)  // PATCH, DELETE, item links
			mux.HandleFunc("/api/agent/discussions", agentHandler.HandleDiscussionsRoot)          // POST — create a discussion thread (#189)
			mux.HandleFunc("/api/agent/discussions/", agentHandler.HandleDiscussionItem)          // POST /{id}/cards, /{id}/conclude (#189)
			mux.HandleFunc("/api/agent/chat/ws", agentHandler.HandleChatWs)                       // WebSocket upgrade & bridge
			mux.HandleFunc("/api/agent/dashboard", agentHandler.HandleDashboard)                  // GET — cross-project cockpit aggregate (read-only)

			// Agents 圆桌脑暴 (design.md slice 1–2): room + 6×tmp seats + R1 chat/brief.
			if rtHandler, rtErr := roundtable.NewHandlerDefaultWithPort(acpxPort); rtErr != nil {
				log.Printf("[server] roundtable init failed: %v", rtErr)
			} else {
				mux.HandleFunc("/api/roundtable/rooms", rtHandler.HandleRoomsRoot)  // POST create
				mux.HandleFunc("/api/roundtable/rooms/", rtHandler.HandleRoomsItem) // GET/POST subroutes
			}

			// Chat-digest: Feishu message sync (sync.db) + value-extraction
			// templates (meta.db v15) + single-batch analysis tasks (run by the
			// scheduler above). Self-wires its own stores; seeds presets and
			// starts a periodic re-sync of every known chat.
			var digestHandler *digest.Handler
			if dh, dErr := digest.NewHandlerDefault(); dErr != nil {
				log.Printf("[server] digest init failed: %v", dErr)
			} else {
				digestHandler = dh
				if err := digestHandler.Seed(); err != nil {
					log.Printf("[server] digest seed: %v", err)
				}
				// NOTE: the standalone 15-min ticker (StartPeriodicSync) is retired.
				// Feishu message sync now runs through the work-order scheduler via the
				// ingest feishu_message task (wired below) — a single scheduler, per
				// the "走工单系统的循环机制" directive.
				mux.HandleFunc("/api/digest/templates", digestHandler.HandleTemplates)     // GET, POST
				mux.HandleFunc("/api/digest/templates/", digestHandler.HandleTemplateItem) // PATCH, DELETE /{id}
				mux.HandleFunc("/api/digest/bindings", digestHandler.HandleBindings)       // GET ?session=, POST, DELETE
				mux.HandleFunc("/api/digest/sync", digestHandler.HandleSync)               // POST {chatId}
				mux.HandleFunc("/api/digest/analyze", digestHandler.HandleAnalyze)         // POST {chatId, workspace}
				mux.HandleFunc("/api/digest/messages", digestHandler.HandleMessages)       // GET ?session=
				// 飞书渠道配置 (Phase 2): browse groups, track/untrack, manual + auto
				// sync (reuses SyncChat watermark + message_id dedup).
				mux.HandleFunc("/api/digest/chats/available", digestHandler.HandleAvailableChats) // GET
				mux.HandleFunc("/api/digest/chats/tracked", digestHandler.HandleTrackedChats)     // GET, POST
				mux.HandleFunc("/api/digest/chats/tracked/", digestHandler.HandleTrackedChatItem) // DELETE, PATCH /{chatId}
				mux.HandleFunc("/api/digest/sync/all", digestHandler.HandleSyncAll)               // POST
				mux.HandleFunc("/api/digest/sync/config", digestHandler.HandleSyncConfig)         // GET, PUT
				mux.HandleFunc("/api/digest/status", digestHandler.HandleStatus)                  // GET
			}

			// 联系人聚合: a user-curated address book (meta.db v16) over channel
			// identities auto-discovered from synced Feishu messages (sync.db).
			// Self-wires its own stores from the default databases.
			if contactsHandler, cErr := contacts.NewHandlerDefault(); cErr != nil {
				log.Printf("[server] contacts init failed: %v", cErr)
			} else {
				mux.HandleFunc("/api/contacts", contactsHandler.HandleContacts)                // GET, POST
				mux.HandleFunc("/api/contacts/channels", contactsHandler.HandleChannels)       // GET ?contactId=&unlinked=1
				mux.HandleFunc("/api/contacts/channels/", contactsHandler.HandleChannelAction) // POST /{id}/link|unlink
				mux.HandleFunc("/api/contacts/discover", contactsHandler.HandleDiscover)       // POST
				mux.HandleFunc("/api/contacts/messages", contactsHandler.HandleMessages)       // GET ?contactId=|sessionId=
				mux.HandleFunc("/api/contacts/sessions", contactsHandler.HandleSessions)       // GET
				mux.HandleFunc("/api/contacts/companies", contactsHandler.HandleCompanies)     // GET tenant→company map
				mux.HandleFunc("/api/contacts/groups/", contactsHandler.HandleGroupMembers)    // GET /{sessionId}/members
				// Local macOS / iCloud channels (sibling syncers of Feishu): iCloud
				// contacts via CardDAV (user's Apple ID + app-specific password, stored
				// locally); iMessage via the local chat.db (needs Full Disk Access).
				// Exact paths outrank the /api/contacts/ prefix below.
				mux.HandleFunc("/api/contacts/icloud/credentials", contactsHandler.HandleICloudCredentials) // GET, POST, DELETE
				mux.HandleFunc("/api/contacts/icloud/sync", contactsHandler.HandleICloudSync)               // POST
				mux.HandleFunc("/api/contacts/imessage/sync", contactsHandler.HandleIMessageSync)           // POST
				// 渠道隐私/同意 + 爬取规则 (per-sub-module consent gate + rules).
				mux.HandleFunc("/api/channels/modules", contactsHandler.HandleChannelModules)     // GET
				mux.HandleFunc("/api/channels/modules/", contactsHandler.HandleChannelModuleItem) // POST/DELETE /{id}/consent, PUT /{id}/rules
				mux.HandleFunc("/api/contacts/", contactsHandler.HandleContactItem)               // PATCH, DELETE /{id}
			}

			// 数据源管理 (data-source management): read-only bronze layer
			// (source_records) — per-(source,kind) rollup for the overview cards and
			// a tabular record list for the 多维表格 detail view.
			if sourcesHandler, sErr := sources.NewHandlerDefault(); sErr != nil {
				log.Printf("[server] sources init failed: %v", sErr)
			} else {
				mux.HandleFunc("/api/sources/summary", sourcesHandler.HandleSummary) // GET
				mux.HandleFunc("/api/sources/records", sourcesHandler.HandleRecords) // GET ?source=&kind=&limit=
			}

			// 数据归一 (data normalization): read-only silver layer (data.db) — the
			// conformed, domain-oriented (联系人/消息/日历/待办) view that cuts across
			// sources. The bronze→silver transform runs on the ingest path; this is
			// the viewer. (POST /api/data/silver/run is registered with ingest below.)
			if dataHandler, dErr := data.NewHandlerDefault(); dErr != nil {
				log.Printf("[server] data (silver) init failed: %v", dErr)
			} else {
				mux.HandleFunc("/api/data/summary", dataHandler.HandleSummary) // GET
				mux.HandleFunc("/api/data/records", dataHandler.HandleRecords) // GET ?domain=&source=&limit=
				// 数据融合 (gold) 只读视图: 跨源归并后的联系人/消息/日历.
				mux.HandleFunc("/api/data/gold/summary", dataHandler.HandleGoldSummary) // GET
				mux.HandleFunc("/api/data/gold/records", dataHandler.HandleGoldRecords) // GET ?domain=&limit=
				// 提拔: 把一条融合待办转成任务 (agent 或 human), 回写 linked_task_id.
				dataHandler.SetSelfBaseURL(selfBaseURL)
				mux.HandleFunc("/api/data/gold/todos/promote", dataHandler.HandlePromoteTodo) // POST {id, workspaceId, assignee}
			}

			mux.HandleFunc("/api/project/local-config", handleProjectLocalConfig) // GET/PUT project-local config json

			// 数据源摄取编排 (ingestion orchestration): CLI 生命周期探针 + 每表爬取
			// 配置 + 工单驱动的立刻/定时同步. Every pull runs as a work-order
			// function task through the scheduler above — this package never grows
			// its own ticker (user directive: 走工单系统的循环机制).
			var ingestHandler *ingest.Handler
			if ih, iErr := ingest.NewHandlerDefault(); iErr != nil {
				log.Printf("[server] ingest init failed: %v", iErr)
			} else {
				ingestHandler = ih
				// Declarative REST connectors: load ~/.1agents/connectors/*.yaml and
				// register each vendor + its REST descriptors BEFORE RegisterFunctions,
				// so the generic sync handler is wired per manifest vendor.
				manifests, mfErr := sources.LoadManifests()
				if mfErr != nil {
					log.Printf("[server] load connector manifests: %v", mfErr)
				}
				for _, m := range manifests {
					sources.RegisterManifest(m)
				}
				ingestHandler.RegisterFunctions()
				if wsPath, pErr := ingestHandler.ProvisionSystemWorkspace(); pErr != nil {
					log.Printf("[server] ingest system workspace: %v", pErr)
				} else {
					ingestHandler.SetDispatcher(ingest.NewDispatcher(taskAPI, tasksStore, wsPath))
				}
				if aErr := ingestHandler.SeedManifestAccounts(manifests); aErr != nil {
					log.Printf("[server] ingest seed manifest accounts: %v", aErr)
				}
				if sErr := ingestHandler.SeedManifestConfigs(manifests); sErr != nil {
					log.Printf("[server] ingest seed manifest configs: %v", sErr)
				}
				ingestHandler.RegisterManifestGovernance(manifests) // generic bronze→silver + viewer tables
				// Standalone governance DAGs (集成/治理解耦): cross-source entity steps.
				if gms, gErr := sources.LoadGovernanceManifests(); gErr != nil {
					log.Printf("[server] load governance manifests: %v", gErr)
				} else {
					ingestHandler.RegisterGovernanceManifests(gms)
				}
				mux.HandleFunc("/api/data/silver/run", ingestHandler.HandleRunSilver) // POST — 手动重新清洗 bronze→silver
				// 数据治理 DAG: 依赖关系 + 执行日志 + 按需重跑。
				mux.HandleFunc("/api/data/governance", ingestHandler.HandleGovernanceDAG)                 // GET — steps + nodes + edges
				mux.HandleFunc("/api/data/governance/runs", ingestHandler.HandleGovernanceRuns)           // GET ?step=&limit=
				mux.HandleFunc("/api/data/governance/run", ingestHandler.HandleGovernanceRun)             // POST ?step=&rebuild= — 重跑整个 DAG 或单步
				mux.HandleFunc("/api/data/governance/table", ingestHandler.HandleGovernanceTable)         // GET ?name=&limit= — 治理输出表下钻
				mux.HandleFunc("/api/data/governance/manifests", ingestHandler.HandleGovernanceManifests) // GET list, POST 热加治理规则/脚本
				mux.HandleFunc("/api/sources/cli/", ingestHandler.CLIHandler().HandleCLI)                 // GET /{tool}/status, POST /{tool}/recheck
				// 账号注册表 (源为中心): 厂家能力 + 每账号 CRUD.
				mux.HandleFunc("/api/sources/vendors", ingestHandler.HandleVendors)       // GET — vendor capability table
				mux.HandleFunc("/api/sources/accounts", ingestHandler.HandleAccounts)     // GET list, POST create
				mux.HandleFunc("/api/sources/accounts/", ingestHandler.HandleAccountItem) // DELETE /{id}
				// Per-source collection config / sync / history — the handlers parse the
				// {source} from the path (feishu / microsoft / google).
				mux.HandleFunc("/api/sources/feishu/collections", ingestHandler.HandleCollections)       // GET, PUT
				mux.HandleFunc("/api/sources/feishu/sync", ingestHandler.HandleSync)                     // POST {kind}
				mux.HandleFunc("/api/sources/feishu/history", ingestHandler.HandleHistory)               // GET
				mux.HandleFunc("/api/sources/feishu/schedules", ingestHandler.HandleSchedules)           // GET — 定时任务触发状态
				mux.HandleFunc("/api/sources/feishu/chats", ingestHandler.HandleChats)                   // GET — cached 群列表 (bronze) + tracked join
				mux.HandleFunc("/api/sources/feishu/chats/members", ingestHandler.HandleChatMembersSync) // POST {chatId} — manual 群成员 roster refresh
				mux.HandleFunc("/api/sources/microsoft/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/microsoft/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/microsoft/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/microsoft/schedules", ingestHandler.HandleSchedules)
				// Microsoft Graph OAuth (PKCE) connect flow — region-aware (大陆/21Vianet).
				mux.HandleFunc("/api/sources/oauth/microsoft/config", ingestHandler.HandleMSOAuthConfig)         // GET ?region= / POST {region,clientId,tenant} — in-UI app registration
				mux.HandleFunc("/api/sources/oauth/microsoft/start", ingestHandler.HandleMSOAuthStart)           // POST {accountId} → {authUrl}
				mux.HandleFunc("/api/sources/oauth/microsoft/callback", ingestHandler.HandleMSOAuthCallback)     // GET  ?code&state (Azure redirect target)
				mux.HandleFunc("/api/sources/oauth/microsoft/status", ingestHandler.HandleMSOAuthStatus)         // GET  ?accountId
				mux.HandleFunc("/api/sources/oauth/microsoft/disconnect", ingestHandler.HandleMSOAuthDisconnect) // POST {accountId}
				mux.HandleFunc("/api/sources/google/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/google/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/google/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/google/schedules", ingestHandler.HandleSchedules)
				mux.HandleFunc("/api/sources/agentmail/collections", ingestHandler.HandleCollections)
				mux.HandleFunc("/api/sources/agentmail/sync", ingestHandler.HandleSync)
				mux.HandleFunc("/api/sources/agentmail/history", ingestHandler.HandleHistory)
				mux.HandleFunc("/api/sources/agentmail/schedules", ingestHandler.HandleSchedules)
				// 推送接入: local agents POST processed records here (transport=push
				// collections). One static prefix serves every push source, so a
				// hot-added push connector needs no new route. See push_http.go.
				mux.HandleFunc("/api/data/push/", ingestHandler.HandlePush) // POST /{source}/{kind} — 落 bronze + 治理
				// 自定义连接器: add/list manifests from the UI (hot-registered, no restart).
				mux.HandleFunc("/api/sources/connectors", ingestHandler.HandleConnectors) // GET list, POST add
				mux.HandleFunc("/api/sources/templates", ingestHandler.HandleTemplates)   // GET embedded templates, POST install
				// Manifest REST sources are served by ONE source-agnostic catch-all (built-in
				// vendors keep their explicit routes above, which win by longest-prefix match).
				// A hot-added vendor needs no new route: /api/sources/{vendor}/{action}.
				mux.HandleFunc("/api/sources/", ingestHandler.HandleManifestRoute)
				if err := ingestHandler.SeedLegacyAccounts(); err != nil {
					log.Printf("[server] ingest seed legacy accounts: %v", err)
				}
			}

			// Cross-wire ingest ⇄ digest: the feishu_message work-order task drives
			// the proven message → unified_messages sync (+ 二度联系人) so the existing
			// message/digest UI stays fresh while bronze holds the raw archive. Then
			// carry the legacy digest auto-sync setting into the new config and arm
			// the recurring tasks — done here (after the callback is set) so the first
			// message-sync run isn't a bronze-only no-op.
			if ingestHandler != nil && digestHandler != nil {
				ingestHandler.SetMessageSync(func(ctx context.Context) error {
					_, _, e := digestHandler.SyncTracked(ctx)
					return e
				})
				if mErr := ingestHandler.MigrateLegacyMessageSync(); mErr != nil {
					log.Printf("[server] ingest migrate legacy message sync: %v", mErr)
				}
				if rErr := ingestHandler.EnsureRecurringForEnabled(); rErr != nil {
					log.Printf("[server] ingest re-arm recurring: %v", rErr)
				}
			}

			// PMO 跨项目对话式需求分发层 (#61) + Workspace Inbox item/deliver routes
			// (#202): Accept reuses Dispatch; deliver is the unified envelope write.
			// Shares the task store (#N numbering / write lock) and one InboxStore.
			if db, dbErr := meta.OpenDefault(); dbErr == nil {
				inboxStore := meta.NewInboxStore(db)
				pmoStore := meta.NewPMOStore(tasksStore, inboxStore)
				mux.HandleFunc("/api/inbox/deliver", meta.InboxDeliverHandler(inboxStore)) // POST deliver
				mux.HandleFunc("/api/inbox/targets", meta.InboxTargetsHandler(pmoStore))   // GET mail targets
				mux.HandleFunc("/api/inbox/", meta.InboxItemHandler(inboxStore, pmoStore)) // GET /{id}; POST /{id}/archive|read|unread|accept
				mux.HandleFunc("/api/pmo/dispatch", meta.PMODispatchHandler(pmoStore))     // GET targets, POST dispatch
			}
		}
	}

	mux.HandleFunc("/api/cc-connect/url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Workspace string `json:"workspace"`
			Theme     string `json:"theme"`
			Lang      string `json:"lang"`
			Path      string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		wsConfig, err := wsHandler.LoadWorkspacesConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var foundWS *workspace.Workspace
		for i := range wsConfig.Workspaces {
			if wsConfig.Workspaces[i].ID == body.Workspace {
				foundWS = &wsConfig.Workspaces[i]
				break
			}
		}

		if foundWS == nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		redirectPath := ""
		if body.Path != "" {
			redirectPath = body.Path
		} else if foundWS.ChatChannel != "" {
			redirectPath = "/chat/" + foundWS.ChatChannel
		} else {
			// #277: project name = workspace name (no __<agent> suffix); the
			// agent type is carried per-channel, not in the project name.
			// Use the shared slug+id-fallback so a non-ASCII workspace name (the
			// default "对话" → "ws" → id "default") targets the SAME project
			// name reconcile created — otherwise the panel 404s on /projects/ws.
			projName := ccconnect.CCProjectName(foundWS.Name, foundWS.ID)
			redirectPath = "/projects/" + projName
		}

		// Normalize language codes from BCP-47 to CC-Connect codes
		normalLang := "zh"
		langLower := strings.ToLower(body.Lang)
		if strings.HasPrefix(langLower, "en") {
			normalLang = "en"
		} else if strings.HasPrefix(langLower, "zh-tw") || strings.HasPrefix(langLower, "zh-hk") {
			normalLang = "zh-TW"
		} else if strings.HasPrefix(langLower, "ja") {
			normalLang = "ja"
		} else if strings.HasPrefix(langLower, "es") {
			normalLang = "es"
		}

		url := fmt.Sprintf("/cc-connect/login?token=%s&redirect=%s&theme=%s&lang=%s",
			ccconnect.ManagementToken,
			url.QueryEscape(redirectPath),
			body.Theme,
			normalLang,
		)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	})

	// ── CC-Connect channel↔agent binding API (#277 Phase 2) ──────────────────
	// GET  ?project=<name>            → list channels + each channel's agent
	// POST {project,index,agent}      → bind/clear a channel's agent (hot reload)
	mux.HandleFunc("/api/cc-connect/channels", ccconnect.ChannelsHandler)
	// Incremental import from an external cc-connect config (default: the shared
	// ~/.cc-connect/config.toml) into 1agents, matched by work_dir path.
	mux.HandleFunc("/api/cc-connect/import", ccconnect.ImportHandler)

	// ── Git API ───────────────────────────────────────────────────────────────
	gitHandler := git.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/git/status", gitHandler.Status)                  // GET
	mux.HandleFunc("/api/git/diff", gitHandler.Diff)                      // GET  ?file=<path>&staged=<bool>
	mux.HandleFunc("/api/git/stage", gitHandler.Stage)                    // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/unstage", gitHandler.Unstage)                // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/discard", gitHandler.Discard)                // POST ?file=<path>
	mux.HandleFunc("/api/git/ai-commit", gitHandler.AICommit)             // POST
	mux.HandleFunc("/api/git/commit", gitHandler.Commit)                  // POST {message:"…"}
	mux.HandleFunc("/api/git/log", gitHandler.Log)                        // GET  ?limit=20
	mux.HandleFunc("/api/git/branches", gitHandler.Branches)              // GET
	mux.HandleFunc("/api/git/checkout", gitHandler.Checkout)              // POST {branch:"…",create:bool}
	mux.HandleFunc("/api/git/push", gitHandler.Push)                      // POST
	mux.HandleFunc("/api/git/pull", gitHandler.Pull)                      // POST
	mux.HandleFunc("/api/git/fetch", gitHandler.Fetch)                    // POST
	mux.HandleFunc("/api/git/worktrees", gitHandler.Worktrees)            // GET
	mux.HandleFunc("/api/git/submodules", gitHandler.Submodules)          // GET
	mux.HandleFunc("/api/git/graph", gitHandler.Graph)                    // GET ?limit=100
	mux.HandleFunc("/api/git/commit-files", gitHandler.CommitFiles)       // GET ?hash=<hash>
	mux.HandleFunc("/api/git/commit-diff", gitHandler.CommitDiff)         // GET ?hash=<hash>&file=<path>
	mux.HandleFunc("/api/git/worktree-status", gitHandler.WorktreeStatus) // GET ?path=<path>
	mux.HandleFunc("/api/git/worktree-diff", gitHandler.WorktreeDiff)     // GET ?path=<path>&file=<file>

	// ── Workspace context API (switches fs + git roots at runtime) ─────────
	ctxHandler := ctxt.NewHandler(fsHandler, gitHandler)
	mux.HandleFunc("/api/context/set", ctxHandler.Set) // POST {"path":"..."}
	mux.HandleFunc("/api/context/get", ctxHandler.Get) // GET
	// ── Terminal API (tmux session management) ────────────────────────────────
	termHandler := terminal.NewHandler(cfg)
	// Create the hidden anchor window (tmux index 0, root shell) at boot so it
	// always exists before any user action and survives a backend restart.
	termHandler.EnsureStartupSession()
	// ttyd starts before the terminal handler creates the persistent tmux
	// session, so refresh Claude's environment once the session exists.
	supervisor.RefreshClaudeTmuxEnvironment(cfg.TmuxSession)
	mux.HandleFunc("/api/terminal/create", termHandler.Create) // POST {workspaceId, cwd}
	mux.HandleFunc("/api/terminal/list", termHandler.List)     // GET
	mux.HandleFunc("/api/terminal/kill", termHandler.Kill)     // POST {windowIndex}
	mux.HandleFunc("/api/terminal/switch", termHandler.Switch) // POST {windowIndex}
	mux.HandleFunc("/api/terminal/rename", termHandler.Rename) // POST {windowName, name}
	mux.HandleFunc("/api/terminal/mouse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			termHandler.GetMouse(w, r)
		} else if r.Method == http.MethodPost {
			termHandler.SetMouse(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ── ttyd reverse proxy ───────────────────────────────────────────────────
	// All WebSocket and HTTP traffic destined for ttyd is forwarded here.
	// The frontend should connect to ws://<host>/ws (not directly to ttyd).
	ttydProxy := gateway.NewTtydProxy(cfg.TtydAddr)
	mux.Handle("/ws", ttydProxy)    // terminal WebSocket stream
	mux.Handle("/token", ttydProxy) // ttyd auth token endpoint

	// ── CC-Connect reverse proxy ─────────────────────────────────────────────
	// Transparently reverse-proxies requests to the local CC-Connect management server
	// under the main HTTPS gateway, resolving LAN protocol security and Mixed Content.
	mux.Handle("/cc-connect/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/assets/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/api/v1/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))

	// ── Bridge WebSocket proxy ──────────────────────────────────────────────
	// Proxies /bridge/ws to the CC-Connect bridge server (dynamic port).
	mux.Handle("/bridge/", gateway.NewBridgeProxy(ccconnect.BridgePort, ccconnect.BridgeToken))

	// ── Module embed scripts (custom elements) ──────────────────────────────
	// Self-contained ESM bundles (HarnessKit / cc-connect web) that register
	// <harnesskit-panel> and <cc-connect-panel>. Production ships them under
	// StaticDir/embed/ (npm @1agents/web, release tarball dist/). Dev falls
	// back to the module build outputs after `build:embed`.
	mux.HandleFunc("/api/embed/harnesskit-embed.js", serveEmbedScript(
		embedBundleCandidates(cfg.StaticDir, "harnesskit-embed.js", []string{
			"modules/HarnessKit/dist-embed/harnesskit-embed.js",
			"../modules/HarnessKit/dist-embed/harnesskit-embed.js",
		}),
	))
	mux.HandleFunc("/api/embed/cc-connect-embed.js", serveEmbedScript(
		embedBundleCandidates(cfg.StaticDir, "cc-connect-embed.js", []string{
			"modules/cc-connect/web/dist-embed/cc-connect-embed.js",
			"../modules/cc-connect/web/dist-embed/cc-connect-embed.js",
		}),
	))
	// Standalone same-origin shell for clients that can only host a web-view
	// (for example the mini-program). It still uses the same authenticated,
	// fail-closed HarnessKit API boundary as the desktop custom element.
	mux.HandleFunc("/extensions/", serveHarnessKitEmbedPage)

	// ── HarnessKit authenticated, fail-closed boundary ─────────────────────
	// The browser never receives the daemon token or a filesystem-authoritative
	// endpoint. Unknown and host-only HarnessKit routes are denied by the
	// version-pinned allowlist in internal/harnesskit.
	if len(harnessKitRuntime) > 0 && harnessKitRuntime[0] != nil {
		harnessKitHandler := harnesskit.NewHandler(harnessKitRuntime[0])
		mux.Handle("/api/harnesskit", harnessKitHandler)
		mux.Handle("/api/harnesskit/", harnessKitHandler)
	}

	// ── Alipay coffee payment module ────────────────────────────────────────
	// Both the embedded page and API stay same-origin behind the main gateway.
	coffeeProxy := gateway.NewCoffeeProxy(cfg.CoffeeAddr)
	mux.Handle("/coffee/", coffeeProxy)
	mux.Handle("/api/coffee/", coffeeProxy)

	// ── Tunnel API (on-demand multi-port tunnel control) ─────────────────────
	tunnelAuth := func(r *http.Request) bool {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + ccconnect.ManagementToken
		return authHeader == expectedAuth || r.URL.Query().Get("token") == ccconnect.ManagementToken
	}

	resolvePort := func(r *http.Request) string {
		if p := r.URL.Query().Get("port"); p != "" {
			return p
		}
		return tunnel.PortFrom(cfg.ListenAddr)
	}

	resolveTimeout := func(r *http.Request) int {
		t := r.URL.Query().Get("timeout")
		if t == "" {
			return 0
		}
		var mins int
		fmt.Sscanf(t, "%d", &mins)
		return mins
	}

	mux.HandleFunc("/api/tunnel/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := resolvePort(r)
		timeout := resolveTimeout(r)
		publicURL, token, err := tunnel.DefaultSupervisor.Start(port, timeout)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"port":  port,
			"url":   publicURL,
			"token": token,
			"link":  fmt.Sprintf("%s/?token=%s", publicURL, token),
		})
	})

	mux.HandleFunc("/api/tunnel/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := r.URL.Query().Get("port")
		if port == "" {
			http.Error(w, "port parameter is required to stop a specific tunnel", http.StatusBadRequest)
			return
		}

		if err := tunnel.DefaultSupervisor.Stop(port); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped", "port": port})
	})

	mux.HandleFunc("/api/tunnel/stop-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		stopped := tunnel.DefaultSupervisor.StopAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "all_stopped",
			"stopped_ports": stopped,
		})
	})

	mux.HandleFunc("/api/tunnel/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tunnels := tunnel.DefaultSupervisor.ListAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  len(tunnels) > 0,
			"tunnels": tunnels,
		})
	})

	// ── System management API (version check + OTA update) ──────────────────
	sysHandler := system.NewHandler()
	mux.HandleFunc("/api/system/version", sysHandler.Version)                         // GET  — current & latest version, has_update flag
	mux.HandleFunc("/api/system/update", sysHandler.Update)                           // POST — trigger OTA update (non-blocking, returns 202)
	mux.HandleFunc("/api/system/update/status", sysHandler.UpdateStatus)              // GET  — real-time update progress log
	mux.HandleFunc(system.ManifestPath, sysHandler.Manifest)                          // GET  — frontend OTA manifest (proxied from GitHub Releases)
	mux.HandleFunc("/api/system/happy/status", sysHandler.HappyStatus)                // GET  — happy daemon status + machine credentials
	mux.HandleFunc("/api/system/happy/daemon/start", sysHandler.HappyDaemonStart)     // POST — start happy daemon
	mux.HandleFunc("/api/system/happy/daemon/stop", sysHandler.HappyDaemonStop)       // POST — stop happy daemon
	mux.HandleFunc("/api/system/happy/pair/start", sysHandler.HappyPairStart)         // POST — begin account-level pairing, returns pairing code
	mux.HandleFunc("/api/system/happy/pair/status", sysHandler.HappyPairStatus)       // GET  — pairing progress (pending/authorized/error)
	mux.HandleFunc("/api/system/happy/ensure-machine", sysHandler.HappyEnsureMachine) // POST — auto-bind machine using local relay-creds
	// 重置本地数据: wipe App data (meta.db/sync.db tables + knowledge/scratch files +
	// workspace-backed cc-connect projects), keep relay pairing identity (~/.happy +
	// relay-creds.json) and provider/model config, re-seed default workspace.
	mux.HandleFunc("/api/system/reset", sysHandler.ResetHandler(wsHandler.EnsureDefaultWorkspace, ccconnect.PurgeWorkspaceProjects)) // POST — reset local data

	// ── Relay client credentials (issue #109) ───────────────────────────────
	mux.HandleFunc("/api/relay/credentials", sysHandler.RelayCredentialsHandler) // GET/POST/DELETE — persist relay account master key

	// ── Device registry + heartbeat + discovery (issue #110) ─────────────────
	mux.HandleFunc("/api/devices", sysHandler.DevicesHandler)            // GET/POST/DELETE — list/register/remove devices
	mux.HandleFunc("/api/devices/heartbeat", sysHandler.DeviceHeartbeat) // POST — refresh a device's lastSeen
	mux.HandleFunc("/api/devices/refresh", sysHandler.DevicesRefresh)    // POST — Tailscale scan + full refresh

	// ── Device proxy routing layer (issue #111) ──────────────────────────────
	// /api/proxy/{deviceId}/... forwards HTTP + WebSocket to a target device
	// resolved via the #110 registry (direct Tailscale/LAN connection). The
	// trailing-slash subtree does not collide with the exact "/api/proxy" web
	// iframe proxy registered below.
	mux.HandleFunc("/api/proxy/", sysHandler.DeviceProxyHandler)

	// ── Access Token API ─────────────────────────────────────────────────────
	mux.HandleFunc("/api/access/status", handleAccessStatus)
	mux.HandleFunc("/api/access/generate", handleAccessGenerate)
	mux.HandleFunc("/api/access/verify", handleAccessVerify)
	mux.HandleFunc("/api/access/revoke", handleAccessRevoke)

	// ── Proxy API ────────────────────────────────────────────────────────────
	// Query form (legacy): /api/proxy?url=http://localhost:3000/TalkingHeadComposition
	// Path form (preferred for SPAs like Remotion):
	//   /api/webproxy/{base64url(origin)}/TalkingHeadComposition
	// so location.pathname's last segment is the composition id even before inject.
	mux.HandleFunc("/api/proxy", handleProxy)
	mux.HandleFunc("/api/webproxy/", handleWebProxy)

	// ── Static frontend assets + task permalink deep links ───────────────────
	// This catch-all must be registered last so it does not shadow the routes
	// above. frontend/dist must contain an index.html for SPA-style navigation.
	//
	// GitHub-style task URLs (/{project}/tasks/{number}) have no file on disk,
	// so the catch-all serves the SPA index for them and lets the frontend
	// resolve the reference (switch project + open the task). A wildcard
	// ServeMux pattern can't express this — a top-level "/{project}/tasks/..."
	// collides with every "/prefix/" subtree route (e.g. /cc-connect/) and
	// panics at registration. So the check lives here in the catch-all, which
	// only sees paths no more-specific route already claimed.
	staticFS := http.FileServer(http.Dir(cfg.StaticDir))
	taskPermalinkRe := regexp.MustCompile(`^/[^/]+/tasks/\d+/?$`)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && taskPermalinkRe.MatchString(r.URL.Path) {
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "index.html"))
			return
		}
		// Big-screen build target (issue #120): the standalone dashboard bundle
		// is emitted as dashboard.html. Serve it at the clean /dashboard path so
		// the big screen has a stable URL distinct from the SPA root.
		if r.Method == http.MethodGet && (r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/") {
			http.ServeFile(w, r, filepath.Join(cfg.StaticDir, "dashboard.html"))
			return
		}
		staticFS.ServeHTTP(w, r)
	})

	return authMiddleware(mux, cfg)
}

// authMiddleware enforces authentication in two layers:
//
//  1. Tunnel auth — when any Cloudflare tunnel is active, the ephemeral session
//     token is required.
//  2. Access token auth — when the user has generated a persistent access token
//     file, all non-localhost requests must present it. Localhost always bypasses.
func authMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Alipay servers cannot present a 1Agents access/tunnel token. This one
		// endpoint is public by design and authenticates the sender through the
		// Alipay SDK signature plus app/order/amount/seller checks downstream.
		if r.Method == http.MethodPost && r.URL.Path == "/api/coffee/notify" {
			next.ServeHTTP(w, r)
			return
		}

		// ── Layer 0: internal loopback bearer ───────────────────────────────
		// Loopback helper subprocesses (e.g. the `1agents project-items` MCP server
		// the AI Project Manager session spawns) present the process-scoped
		// internal token. Accept it only from localhost so the bypass can never
		// be reached over the tunnel, then skip both auth layers below.
		if isLocalhost(r) {
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				bearer := strings.TrimPrefix(authHeader, "Bearer ")
				sessionID := r.Header.Get("X-OneAgents-Session-ID")
				if bearer == localtoken.Token || localtoken.ValidateSessionToken(sessionID, bearer) {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// ── Layer 1: Tunnel session auth ────────────────────────────────────
		if tunnel.DefaultSupervisor.HasAnyActive() {
			// Bypass tunnel auth for tunnel control APIs
			if !strings.HasPrefix(r.URL.Path, "/api/tunnel/") {
				authenticated := false
				var matchedToken string

				checkToken := func(tok string) bool {
					if tok != "" && tunnel.DefaultSupervisor.ValidateToken(tok) {
						matchedToken = tok
						return true
					}
					return false
				}

				if tokenParam := r.URL.Query().Get("token"); tokenParam != "" {
					if checkToken(tokenParam) {
						authenticated = true
						http.SetCookie(w, &http.Cookie{
							Name:     "ra_session_token",
							Value:    matchedToken,
							Path:     "/",
							HttpOnly: true,
							Secure:   true,
							SameSite: http.SameSiteLaxMode,
						})
					}
				}

				if !authenticated {
					authHeader := r.Header.Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						if checkToken(strings.TrimPrefix(authHeader, "Bearer ")) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					if cookie, err := r.Cookie("ra_session_token"); err == nil {
						if checkToken(cookie.Value) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error": "Unauthorized: Ephemeral session token required. Please scan the authorized QR code or click the secure link."}`))
					return
				}
			}
		}

		// ── Layer 2: Access token auth ─────────────────────────────────────
		if !auth.TokenExists() {
			next.ServeHTTP(w, r)
			return
		}

		// Access token API endpoints manage their own auth
		if strings.HasPrefix(r.URL.Path, "/api/access/") {
			next.ServeHTTP(w, r)
			return
		}

		// Localhost always bypasses
		if isLocalhost(r) {
			next.ServeHTTP(w, r)
			return
		}

		storedToken, _ := auth.LoadToken()
		if storedToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		accessAuthenticated := false

		// Mechanism A: ?access_token= query param
		if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
			accessAuthenticated = true
		}

		// Mechanism B: Authorization: Bearer <token> (also checks access token)
		if !accessAuthenticated {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					accessAuthenticated = true
				}
			}
		}

		// Mechanism C: ra_access_token cookie
		if !accessAuthenticated {
			if cookie, err := r.Cookie("ra_access_token"); err == nil {
				if cookie.Value == storedToken {
					accessAuthenticated = true
				}
			}
		}

		if accessAuthenticated {
			// Refresh long-lived cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "ra_access_token",
				Value:    storedToken,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   365 * 24 * 3600,
			})
			next.ServeHTTP(w, r)
			return
		}

		// Not authenticated — reject API calls, let page requests through
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"access_token_required","message":"An access token is required for non-localhost access."}`))
			return
		}

		// Page request: let SPA load; it will call /api/access/status and show gate
		next.ServeHTTP(w, r)
	})
}

// isLocalhost reports whether the request comes from loopback or a private LAN
// address (RFC 1918 / unique-local / link-local). Access-token auth treats these
// the same as localhost so opening http://192.168.x.x:8085/ on the home network
// gets the happy-cli operator bypass without a token. Public RemoteAddrs still
// require the access token (tunnel session auth remains a separate layer).
func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// acpxBridgePort is the port the 1acp bridge-server listens on and the backend
// dials. Defaults to 38082, overridable via ACPX_PORT — the same env var the
// supervisor passes to the spawned bridge-server — so an isolated second
// instance can run without colliding with a primary one.
func acpxBridgePort() int {
	if v := os.Getenv("ACPX_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			return p
		}
	}
	return 38082
}

// ── Access Token Handlers ───────────────────────────────────────────────────────

func handleAccessStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	required := auth.TokenExists()
	authenticated := true

	if required && !isLocalhost(r) {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			authenticated = false

			if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
				authenticated = true
			}
			if !authenticated {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
						authenticated = true
					}
				}
			}
			if !authenticated {
				if cookie, err := r.Cookie("ra_access_token"); err == nil {
					if cookie.Value == storedToken {
						authenticated = true
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{
		"required":      required,
		"authenticated": authenticated,
	})
}

func handleAccessGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isLocalhost(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token generation is only allowed from localhost."})
		return
	}

	token := tunnel.GenerateRandomToken()
	if err := auth.SaveToken(token); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"token":   token,
		"message": "Access token generated. Save it now — it will not be shown again.",
	})
}

func handleAccessVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Invalid request body."})
		return
	}

	if body.Token == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Token is required."})
		return
	}

	storedToken, err := auth.LoadToken()
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	if body.Token != storedToken {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "无效的访问令牌。"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ra_access_token",
		Value:    storedToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func handleAccessRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow localhost or already-authenticated callers
	allowed := isLocalhost(r)
	if !allowed {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			if cookie, err := r.Cookie("ra_access_token"); err == nil && cookie.Value == storedToken {
				allowed = true
			}
			if !allowed {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					allowed = true
				}
			}
		}
	}

	if !allowed {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token revocation requires localhost or authenticated access."})
		return
	}

	if err := auth.DeleteToken(); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Access token revoked."})
}

// proxyHopHeaders are hop-by-hop headers that must not be forwarded (RFC 7230).
var proxyHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"host":                true,
}

// proxyStripRequestHeaders must not be copied from the browser request.
// Accept-Encoding is critical: if the client sets it, Go's Transport will NOT
// transparently decompress gzip, but we always strip Content-Encoding on the
// way out — leaving the browser with raw gzip bytes (JSON.parse sees "�{...").
var proxyStripRequestHeaders = map[string]bool{
	"accept-encoding": true,
	// Browser Origin/Referer point at the 1agents host (e.g. :38080); rewrite
	// them to the target origin below so apps that check Origin don't 4xx/5xx.
	"origin":  true,
	"referer": true,
}

// handleProxy is the legacy query form: /api/proxy?url=<absolute-url>.
// Prefer handleWebProxy for path-routed SPAs (Remotion).
func handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}
	proxyToTarget(w, r, targetURL)
}

// handleWebProxy is the path form used by the built-in browser:
//
//	/api/webproxy/{base64url(origin)}/{path...}?query
//
// Example for Remotion:
//
//	/api/webproxy/aHR0cDovL2xvY2FsaG9zdDozMDAw/TalkingHeadComposition
//
// Remotion derives composition id from the last path segment
// (deriveCanvasContentFromRoute), so this shape works even before inject
// clean-path runs — unlike /api/proxy?url=… where pathname is always "/api/proxy".
func handleWebProxy(w http.ResponseWriter, r *http.Request) {
	targetURL, err := parseWebProxyPath(r.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	proxyToTarget(w, r, targetURL)
}

// parseWebProxyPath decodes /api/webproxy/{b64origin}/{path...} into an absolute URL.
func parseWebProxyPath(u *url.URL) (string, error) {
	const prefix = "/api/webproxy/"
	if !strings.HasPrefix(u.Path, prefix) {
		return "", fmt.Errorf("not a webproxy path")
	}
	rest := strings.TrimPrefix(u.Path, prefix)
	if rest == "" {
		return "", fmt.Errorf("missing origin token")
	}
	b64 := rest
	pathPart := "/"
	if i := strings.Index(rest, "/"); i >= 0 {
		b64 = rest[:i]
		pathPart = rest[i:] // includes leading /
		if pathPart == "" {
			pathPart = "/"
		}
	}
	originBytes, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		// Accept standard base64url with padding too.
		originBytes, err = base64.URLEncoding.DecodeString(b64)
		if err != nil {
			return "", fmt.Errorf("invalid origin token: %w", err)
		}
	}
	origin := string(originBytes)
	ou, err := url.Parse(origin)
	if err != nil || ou.Scheme == "" || ou.Host == "" {
		return "", fmt.Errorf("invalid origin %q", origin)
	}
	target := strings.TrimRight(origin, "/") + pathPart
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		target += "#" + u.Fragment
	}
	return target, nil
}

// encodeWebProxyPath builds /api/webproxy/{b64origin}{pathname} for tests/helpers.
func encodeWebProxyPath(target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("target must be absolute")
	}
	origin := u.Scheme + "://" + u.Host
	b64 := base64.RawURLEncoding.EncodeToString([]byte(origin))
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	out := "/api/webproxy/" + b64 + path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, nil
}

// proxyToTarget reverse-proxies the browser request to targetURL.
// HTML responses get base + bootstrap inject (clean-path for Remotion, network rewrites).
func proxyToTarget(w http.ResponseWriter, r *http.Request, targetURL string) {
	// CORS preflight from the iframe (same-origin normally, but keep cheap).
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		if w.Header().Get("Access-Control-Allow-Headers") == "" {
			w.Header().Set("Access-Control-Allow-Headers", "*")
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var body io.Reader
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		body = r.Body
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward client headers except hop-by-hop / encoding / origin.
	for k, vv := range r.Header {
		lk := strings.ToLower(k)
		if proxyHopHeaders[lk] || proxyStripRequestHeaders[lk] {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	// Ensure Go Transport may negotiate+decompress gzip (see proxyStripRequestHeaders).
	req.Header.Del("Accept-Encoding")

	if u, err := url.Parse(targetURL); err == nil {
		req.Host = u.Host
		targetOrigin := u.Scheme + "://" + u.Host
		req.Header.Set("Origin", targetOrigin)
		req.Header.Set("Referer", targetOrigin+"/")
	}

	// No overall Timeout — SSE and other long-lived streams must not be cut off.
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html")
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	// HTML: buffer + inject base/script so subsequent navigations stay proxied.
	// Non-HTML (JSON, assets, SSE): stream through so EventSource works.
	if isHTML {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Prefer the URL the client asked for (keeps composition path even if
		// upstream redirected); fall back to final request URL.
		pageURL := targetURL
		if resp.Request != nil && resp.Request.URL != nil {
			// Keep path from target if upstream only added trailing slash noise.
			final := resp.Request.URL.String()
			if tu, err := url.Parse(targetURL); err == nil && tu.Path != "" && tu.Path != "/" {
				pageURL = targetURL
			} else {
				pageURL = final
			}
		}
		bodyBytes = injectProxyBootstrap(bodyBytes, pageURL)
		copyProxyResponseHeaders(w, resp.Header, true)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(bodyBytes)
		return
	}

	copyProxyResponseHeaders(w, resp.Header, false)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if isSSE {
		// Discourage reverse proxies / browsers from buffering the event stream.
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(resp.StatusCode)

	if flusher, ok := w.(http.Flusher); ok {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}
	_, _ = io.Copy(w, resp.Body)
}

// copyProxyResponseHeaders copies upstream headers onto the client response,
// dropping frame-busting policy and stale hop length/encoding headers.
// When buffered is true, Content-Length/Encoding are always stripped (body
// may have been rewritten). When streaming, strip only when they would lie
// after Go's transport auto-decompression (encoding already removed by net/http).
func copyProxyResponseHeaders(w http.ResponseWriter, src http.Header, buffered bool) {
	for k, v := range src {
		lowerK := strings.ToLower(k)
		if lowerK == "x-frame-options" || lowerK == "content-security-policy" || lowerK == "csp" ||
			lowerK == "content-length" || lowerK == "content-encoding" || lowerK == "transfer-encoding" {
			continue
		}
		if buffered && lowerK == "content-length" {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
}

// injectProxyBootstrap prepends <base> + rewrite scripts at the document start
// so they run before any app JS (Remotion reads location.pathname at boot).
// Network stays on the host proxy; SPA path routing uses a clean-path trick
// (see proxyInjectScript) because Location.prototype cannot be spoofed in Chromium.
func injectProxyBootstrap(body []byte, actualURL string) []byte {
	htmlStr := string(body)

	// Escape for embedding inside a JS double-quoted string.
	jsTarget := strings.ReplaceAll(actualURL, `\`, `\\`)
	jsTarget = strings.ReplaceAll(jsTarget, `"`, `\"`)
	jsTarget = strings.ReplaceAll(jsTarget, "\n", `\n`)
	jsTarget = strings.ReplaceAll(jsTarget, "\r", `\r`)
	jsTarget = strings.ReplaceAll(jsTarget, "</", "<\\/")

	// <base> must be origin root (with trailing slash), NOT the full composition
	// URL — otherwise relative asset resolution breaks under Remotion.
	baseHref := actualURL
	if u, err := url.Parse(actualURL); err == nil && u.Scheme != "" && u.Host != "" {
		baseHref = u.Scheme + "://" + u.Host + "/"
	}

	// Prepend: guarantees execution before any target script.
	// Script history.replaceState's onto the target pathname so Remotion's
	// getRoute()/pathname.replace('/','') sees "TalkingHeadComposition" not "api/proxy".
	baseTag := `<base href="` + baseHref + `">`
	scriptTag := strings.Replace(proxyInjectScript, "__TARGET_URL__", jsTarget, 1)
	return []byte(baseTag + scriptTag + htmlStr)
}

// proxyInjectScript — built-in browser iframe bootstrap.
//
// Remotion Studio (see getRoute / CanvasOrLoading):
//
//	compositionId = location.pathname last segment  OR  pathname.replace('/','')
//
// So pathname "/api/proxy" → id "api/proxy" → "Composition with ID api/proxy not found."
//
// Preferred load shape (NEVER rewrite history to bare "/TalkingHeadComposition"):
//
//	/api/webproxy/{b64(origin)}/TalkingHeadComposition
//
// Remotion takes composition id from the LAST path segment, so this works.
// Bare paths on the 1agents host 404 on reload (static catch-all).
//
// Placeholders:
//
//	__TARGET_URL__ — absolute URL of the page being proxied (set at inject time)
const proxyInjectScript = `
<script>
(function() {
  var virtualHref = "__TARGET_URL__";

  function b64urlEncode(str) {
    var s = btoa(unescape(encodeURIComponent(str)));
    return s.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
  }
  function b64urlDecode(s) {
    s = String(s).replace(/-/g, '+').replace(/_/g, '/');
    while (s.length % 4) s += '=';
    try { return decodeURIComponent(escape(atob(s))); } catch(e) { return atob(s); }
  }

  // Decode /api/webproxy/{b64origin}/{path...}
  function parseWebProxyPath(href) {
    try {
      var u = new URL(href || window.location.href);
      var prefix = '/api/webproxy/';
      if (u.pathname.indexOf(prefix) !== 0) return null;
      var rest = u.pathname.slice(prefix.length);
      if (!rest) return null;
      var slash = rest.indexOf('/');
      var b64 = slash < 0 ? rest : rest.slice(0, slash);
      var path = slash < 0 ? '/' : rest.slice(slash);
      if (!path) path = '/';
      return b64urlDecode(b64) + path + u.search + u.hash;
    } catch(e) { return null; }
  }

  function parseProxyQuery(href) {
    try {
      var urlObj = new URL(href || window.location.href);
      if (urlObj.pathname === '/api/proxy') {
        var target = urlObj.searchParams.get('url');
        if (target) return target;
      }
    } catch(e) {}
    return null;
  }

  try {
    if (!virtualHref || virtualHref.indexOf('__TARGET') === 0) {
      virtualHref = parseWebProxyPath() || parseProxyQuery() || window.location.href;
    }
    virtualHref = new URL(virtualHref).href;
  } catch(e) {
    virtualHref = window.location.href;
  }

  var targetOrigin;
  try { targetOrigin = new URL(virtualHref).origin; } catch(e) { targetOrigin = window.location.origin; }

  function getVirtual() {
    try { return new URL(virtualHref); } catch(e) { return new URL(targetOrigin + '/'); }
  }
  function setVirtual(href) {
    try {
      var next = new URL(href, getVirtual().href);
      // If app navigates using bare path on 1agents origin, map back to target.
      if (next.origin === window.location.origin && next.pathname.indexOf('/api/webproxy/') !== 0 && next.pathname !== '/api/proxy') {
        virtualHref = targetOrigin + next.pathname + next.search + next.hash;
      } else if (next.origin === targetOrigin || next.protocol === 'http:' || next.protocol === 'https:') {
        if (next.origin === targetOrigin) virtualHref = next.href;
        else if (next.origin === window.location.origin) {
          var decoded = parseWebProxyPath(next.href);
          if (decoded) virtualHref = decoded;
        } else {
          virtualHref = next.href;
        }
      }
    } catch(e) {}
  }

  // History + full navigations ALWAYS stay on /api/webproxy/... so reload works
  // (bare /TalkingHeadComposition is not a 1agents route → 404).
  function buildWebProxyURL(absoluteHref) {
    var u = new URL(absoluteHref, getVirtual().href);
    if (u.origin === window.location.origin) {
      var decoded = parseWebProxyPath(u.href);
      if (decoded) u = new URL(decoded);
      else if (u.pathname !== '/api/proxy') {
        u = new URL(targetOrigin + u.pathname + u.search + u.hash);
      }
    }
    var path = u.pathname || '/';
    return window.location.origin + '/api/webproxy/' + b64urlEncode(u.origin) + path + u.search + u.hash;
  }

  function toHistoryURL(href) {
    // Absolute same-origin webproxy URL (base tag must not rewrite this).
    return buildWebProxyURL(href);
  }

  function toProxiedReload(href) {
    return buildWebProxyURL(href);
  }

  function isOurProxyPath(pathname) {
    return pathname === '/api/proxy' || pathname.indexOf('/api/webproxy/') === 0;
  }

  function toProxiedNetwork(url) {
    try {
      var resolved = new URL(url, getVirtual().href);
      var proxyOrigin = window.location.origin;

      if (resolved.origin === proxyOrigin && isOurProxyPath(resolved.pathname)) {
        return resolved.href;
      }
      if (resolved.origin === targetOrigin) {
        return buildWebProxyURL(resolved.href);
      }
      if (resolved.origin === proxyOrigin && !isOurProxyPath(resolved.pathname)) {
        if (resolved.pathname.indexOf('/api/') === 0 && resolved.pathname.indexOf('/api/webproxy/') !== 0) {
          return resolved.href;
        }
        return buildWebProxyURL(targetOrigin + resolved.pathname + resolved.search + resolved.hash);
      }
      if (resolved.origin !== proxyOrigin) {
        return buildWebProxyURL(resolved.href);
      }
      return resolved.href;
    } catch(e) {
      return url;
    }
  }

  var originalPushState = window.history.pushState.bind(window.history);
  var originalReplaceState = window.history.replaceState.bind(window.history);

  // Normalize legacy ?url= loads onto path-form webproxy (reload-safe).
  try {
    if (window.location.pathname === '/api/proxy' || !isOurProxyPath(window.location.pathname)) {
      var desired = buildWebProxyURL(virtualHref);
      if (window.location.href !== desired) {
        originalReplaceState.call(window.history, window.history.state, '', desired);
      }
    }
  } catch(e) {
    console.warn('[1agents proxy] history normalize failed', e);
  }

  function notifyParent(url) {
    try {
      window.parent.postMessage({ type: 'iframe_navigate', url: url || virtualHref }, '*');
    } catch(e) {}
  }
  notifyParent(virtualHref);

  window.addEventListener('popstate', function() {
    try {
      var decoded = parseWebProxyPath(window.location.href);
      if (decoded) virtualHref = decoded;
      else if (!isOurProxyPath(window.location.pathname)) {
        virtualHref = targetOrigin + window.location.pathname + window.location.search + window.location.hash;
      }
    } catch(e) {}
    notifyParent(virtualHref);
  });

  document.addEventListener('click', function(e) {
    var a = e.target && e.target.closest ? e.target.closest('a') : null;
    if (!a || !a.href) return;
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    if (a.target && a.target !== '' && a.target !== '_self') return;
    try {
      var resolved = new URL(a.href, getVirtual().href);
      if (resolved.origin === targetOrigin || resolved.origin === window.location.origin) {
        e.preventDefault();
        setVirtual(resolved.href);
        window.location.href = toProxiedReload(virtualHref);
      }
    } catch(err) {}
  }, true);

  document.addEventListener('submit', function(e) {
    var form = e.target;
    if (!form || !form.action) return;
    if ((form.method || 'get').toLowerCase() !== 'get') return;
    e.preventDefault();
    try {
      var url = new URL(form.action, getVirtual().href);
      var formData = new FormData(form);
      for (var pair of formData.entries()) {
        url.searchParams.set(pair[0], pair[1]);
      }
      setVirtual(url.href);
      window.location.href = toProxiedReload(virtualHref);
    } catch(err) {}
  }, true);

  window.history.pushState = function(state, title, url) {
    try {
      if (url !== undefined && url !== null && String(url) !== '') {
        var resolvedUrl = new URL(String(url), getVirtual().href).href;
        setVirtual(resolvedUrl);
        originalPushState(state, title, toHistoryURL(virtualHref));
        notifyParent(virtualHref);
        return;
      }
    } catch (e) {
      console.warn('[1agents proxy] pushState', e);
    }
    return originalPushState(state, title, url);
  };

  window.history.replaceState = function(state, title, url) {
    try {
      if (url !== undefined && url !== null && String(url) !== '') {
        var resolvedUrl = new URL(String(url), getVirtual().href).href;
        setVirtual(resolvedUrl);
        originalReplaceState(state, title, toHistoryURL(virtualHref));
        notifyParent(virtualHref);
        return;
      }
    } catch (e) {
      console.warn('[1agents proxy] replaceState', e);
    }
    return originalReplaceState(state, title, url);
  };

  if (window.fetch) {
    var originalFetch = window.fetch;
    window.fetch = function(input, init) {
      try {
        var url;
        if (typeof input === 'string') url = input;
        else if (input instanceof URL) url = input.href;
        else if (input && input.url) url = input.url;
        if (url) {
          var proxied = toProxiedNetwork(url);
          var resolved = new URL(url, getVirtual().href).href;
          if (proxied !== resolved && proxied !== url) {
            if (typeof input === 'string') input = proxied;
            else if (input instanceof URL) input = new URL(proxied);
            else if (typeof Request !== 'undefined' && input instanceof Request) input = new Request(proxied, input);
            else if (input && input.url) input = new Request(proxied, input);
          }
        }
      } catch (e) {
        console.warn('[1agents proxy] fetch', e);
      }
      return originalFetch.call(this, input, init);
    };
  }

  if (window.XMLHttpRequest) {
    var originalOpen = window.XMLHttpRequest.prototype.open;
    window.XMLHttpRequest.prototype.open = function(method, url) {
      try {
        if (url) {
          var proxied = toProxiedNetwork(url);
          var resolved = new URL(String(url), getVirtual().href).href;
          if (proxied !== resolved && proxied !== String(url)) arguments[1] = proxied;
        }
      } catch (e) {
        console.warn('[1agents proxy] xhr', e);
      }
      return originalOpen.apply(this, arguments);
    };
  }

  if (window.EventSource) {
    var OriginalEventSource = window.EventSource;
    function ProxiedEventSource(url, config) {
      var finalUrl = url;
      try {
        if (url) {
          var proxied = toProxiedNetwork(url);
          var resolved = new URL(String(url), getVirtual().href).href;
          if (proxied !== resolved && proxied !== String(url)) finalUrl = proxied;
        }
      } catch (e) {
        console.warn('[1agents proxy] EventSource', e);
      }
      return new OriginalEventSource(finalUrl, config);
    }
    ProxiedEventSource.prototype = OriginalEventSource.prototype;
    ProxiedEventSource.CONNECTING = OriginalEventSource.CONNECTING;
    ProxiedEventSource.OPEN = OriginalEventSource.OPEN;
    ProxiedEventSource.CLOSED = OriginalEventSource.CLOSED;
    window.EventSource = ProxiedEventSource;
  }

  // Workers cannot load cross-origin scripts (base → :3000 while page is :38080).
  // Route worker scripts through same-origin webproxy.
  function wrapWorkerCtor(Orig) {
    if (!Orig) return Orig;
    function ProxiedWorker(scriptURL, options) {
      var finalUrl = scriptURL;
      try {
        if (scriptURL) {
          var proxied = toProxiedNetwork(String(scriptURL));
          var resolved = new URL(String(scriptURL), getVirtual().href).href;
          if (proxied !== resolved && proxied !== String(scriptURL)) finalUrl = proxied;
        }
      } catch (e) {
        console.warn('[1agents proxy] Worker', e);
      }
      return new Orig(finalUrl, options);
    }
    ProxiedWorker.prototype = Orig.prototype;
    try {
      Object.keys(Orig).forEach(function(k) {
        try { ProxiedWorker[k] = Orig[k]; } catch(e) {}
      });
    } catch(e) {}
    return ProxiedWorker;
  }
  try {
    if (window.Worker) window.Worker = wrapWorkerCtor(window.Worker);
    if (window.SharedWorker) window.SharedWorker = wrapWorkerCtor(window.SharedWorker);
  } catch(e) {}

  if (navigator.serviceWorker) {
    try {
      navigator.serviceWorker.register = function() {
        return Promise.reject(new Error('Service Worker disabled under 1agents proxy'));
      };
      if (navigator.serviceWorker.getRegistrations) {
        navigator.serviceWorker.getRegistrations().then(function(regs) {
          regs.forEach(function(r) { try { r.unregister(); } catch(e) {} });
        });
      }
    } catch(e) {}
  }
})();
</script>
`

// serveEmbedScript returns an http.HandlerFunc that serves a single
// submodule embed bundle. The handler resolves the file lazily on each
// request, so a submodule that is built *after* 1agents has started
// becomes available without a restart. The first existing path wins.
//
// If none of the candidates exist the handler returns 404 with a hint
// telling the operator how to produce the bundle. This is intentional:
// silently shadowing the static catch-all would make "iframe doesn't
// load" look like "module registration failed", which is much harder
// to diagnose.
// appregistryItemHandler routes /api/apps/{id}/enable and /api/apps/{id}/disable
// to the appropriate appregistry handler. The mux prefix route /api/apps/ catches
// both sub-paths; we dispatch by suffix.
func appregistryItemHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/enable"):
		appregistry.HandleEnable(w, r)
	case strings.HasSuffix(path, "/disable"):
		appregistry.HandleDisable(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// shellRegistryItemHandler routes the /api/shells/ prefix:
//
//	POST /api/shells/{id}/enable | /api/shells/{id}/disable
//	PUT  /api/shells/default | /api/shells/preference
//	GET  /api/shells/composition?shell=<id>
func shellRegistryItemHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/enable"):
		appregistry.HandleShellEnable(w, r)
	case strings.HasSuffix(path, "/disable"):
		appregistry.HandleShellDisable(w, r)
	case strings.HasSuffix(path, "/default"):
		appregistry.HandleShellDefault(w, r)
	case strings.HasSuffix(path, "/preference"):
		appregistry.HandleShellPreference(w, r)
	case strings.HasSuffix(path, "/composition"):
		appregistry.HandleShellComposition(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// projectConfigHandler routes GET /api/project/config and PUT /api/project/config.
func projectConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspace.HandleProjectConfigGet(w, r)
	case http.MethodPut:
		workspace.HandleProjectConfigPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// embedBundleCandidates returns filesystem paths to try for a named embed
// ESM file (e.g. harnesskit-embed.js). Order:
//  1. -static dir (production: @1agents/web/dist/embed/…)
//  2. release layout next to the 1agents binary (…/dist/embed/…)
//  3. monorepo modules/*/dist-embed (dev)
func embedBundleCandidates(staticDir, fileName string, monorepoRels []string) []string {
	var c []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		// Normalize later via Abs in the handler; de-dupe on raw string.
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		c = append(c, p)
	}

	if staticDir != "" {
		add(filepath.Join(staticDir, "embed", fileName))
		add(filepath.Join(staticDir, fileName))
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(filepath.Join(exeDir, "dist", "embed", fileName))
		add(filepath.Join(exeDir, "..", "dist", "embed", fileName))
		add(filepath.Join(exeDir, "embed", fileName))
	}

	for _, rel := range monorepoRels {
		add(rel)
	}
	return c
}

func serveEmbedScript(candidates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET — these are static assets; anything else is a bug.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, c := range candidates {
			abs, err := filepath.Abs(c)
			if err != nil {
				continue
			}
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache")
				http.ServeFile(w, r, abs)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w,
			"embed bundle not found; tried: %s\n"+
				"Production: ensure @1agents/web includes dist/embed/*.js (scripts/build-module-embeds.sh).\n"+
				"Dev: run `npm run build:embed` in modules/HarnessKit and modules/cc-connect/web, or ./scripts/build-module-embeds.sh",
			strings.Join(candidates, ", "),
		)
	}
}

func serveHarnessKitEmbedPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	theme := r.URL.Query().Get("theme")
	if theme != "dark" && theme != "light" {
		theme = "system"
	}
	language := r.URL.Query().Get("lang")
	if language != "zh" {
		language = "en"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>1agents Extensions</title><style>html,body,harnesskit-panel{display:block;width:100%%;height:100%%;margin:0}</style>
<script type="module" src="/api/embed/harnesskit-embed.js"></script></head>
<body><harnesskit-panel theme="%s" language="%s"></harnesskit-panel></body></html>`,
		language, theme, language)
}

// handleProjectLocalConfig reads/writes a project-local config blob at
// <workspacePath>/.1agents/project_config.json. Generic passthrough — the
// frontend owns the schema (e.g. hiddenTabs). Server-persisted and travels with
// the project directory, so it works cross-device without a DB.
func handleProjectLocalConfig(w http.ResponseWriter, r *http.Request) {
	ws := r.URL.Query().Get("workspacePath")
	if ws == "" {
		http.Error(w, "workspacePath required", http.StatusBadRequest)
		return
	}
	path := filepath.Join(ws, ".1agents", "project_config.json")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var incoming map[string]any
		if err := json.Unmarshal(body, &incoming); err != nil {
			http.Error(w, "body must be a JSON object: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Shallow-merge into the existing file so independent writers (e.g. tab
		// visibility vs project config) each own their own top-level keys without
		// clobbering the others.
		merged := map[string]any{}
		if existing, rerr := os.ReadFile(path); rerr == nil {
			_ = json.Unmarshal(existing, &merged)
		}
		for k, v := range incoming {
			merged[k] = v
		}
		out, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
