package feishu

// catalog.go is the declarative surface of the Feishu ingestion base: one
// CollectionDescriptor per open-platform list endpoint the Puller can crawl.
// "全面兼容飞书 CLI" is achieved by growing this table, not by writing new pull
// code — the generic FeishuPuller (puller.go) reads a descriptor, calls RawAPI,
// and lands每一条 raw item verbatim into bronze (source_records).
//
// Scope note: raw ingestion only. Governance (bronze→gold) is a separate step.

// CollectionDescriptor describes how to crawl one Feishu data kind.
type CollectionDescriptor struct {
	// Kind is the source_records.kind discriminator (e.g. "feishu_chat").
	Kind string
	// Domain groups kinds for the UI (通讯录/IM/日历/审批/考勤/…).
	Domain string
	// Label is the human name shown in the 数据源 config UI.
	Label string
	// Endpoint / Method are the lark-cli `api` args.
	Endpoint string
	Method   string
	// BaseParams are always-sent query params (page_size is filled from config).
	BaseParams map[string]string
	// ItemPath is the dotted path to the items array in the response
	// (default "data.items"). Endpoints that nest differently (e.g. calendars
	// under data.calendar_list) override it.
	ItemPath string
	// UIDField is the item field that is the stable per-record id.
	UIDField string
	// CursorFlavor selects the incremental strategy:
	//   "timestamp"  → TimeParam carries a lower-bound epoch-seconds watermark
	//   "page_token" → no time filter; full re-crawl each run, ETag dedups
	//   ""           → same as page_token (no incremental support yet)
	CursorFlavor string
	// TimeParam is the request param that bounds the lower time edge (timestamp
	// flavor only), e.g. "start_time".
	TimeParam string
	// TimeItemField is the item field holding the record's timestamp, used to
	// advance the watermark (timestamp flavor only), e.g. "create_time".
	TimeItemField string
	// TimeMs reports whether TimeItemField is epoch-milliseconds (true) or
	// seconds (false). Feishu message create_time is ms while the request
	// start_time is seconds — this centralizes that easy-to-miss conversion.
	TimeMs bool
	// PerChat marks message-family kinds whose collection is one chat: Discover
	// expands them across the tracked-chat set instead of a single collection.
	PerChat bool
	// Implemented gates whether Discover surfaces the kind. Descriptors for
	// endpoints whose list semantics are not wired yet (department traversal,
	// approval-code fan-out) ship as documentation with Implemented=false.
	Implemented bool
}

// Source is the stable source discriminator for every Feishu collection.
const Source = "feishu"

// Domain labels (also the UI grouping keys).
const (
	DomainContacts   = "contacts"
	DomainIM         = "im"
	DomainCalendar   = "calendar"
	DomainApproval   = "approval"
	DomainAttendance = "attendance"
)

// catalog is the first-wave descriptor table. Kinds are namespaced with a
// "feishu_" prefix so bronze rows never collide with another source's kind.
var catalog = []CollectionDescriptor{
	// ── IM / 消息 ─────────────────────────────────────────────────────────
	{
		Kind: "feishu_chat", Domain: DomainIM, Label: "群会话",
		Endpoint: "/open-apis/im/v1/chats", Method: "GET",
		BaseParams: map[string]string{}, ItemPath: "data.items", UIDField: "chat_id",
		CursorFlavor: "page_token", Implemented: true,
	},
	{
		Kind: "feishu_message", Domain: DomainIM, Label: "群消息",
		Endpoint: "/open-apis/im/v1/messages", Method: "GET",
		BaseParams: map[string]string{"container_id_type": "chat", "sort_type": "ByCreateTimeAsc"},
		ItemPath:   "data.items", UIDField: "message_id",
		CursorFlavor: "timestamp", TimeParam: "start_time",
		TimeItemField: "create_time", TimeMs: true,
		PerChat: true, Implemented: true,
	},

	// ── 日历 ─────────────────────────────────────────────────────────────
	{
		Kind: "feishu_calendar", Domain: DomainCalendar, Label: "日历",
		Endpoint: "/open-apis/calendar/v4/calendars", Method: "GET",
		BaseParams: map[string]string{}, ItemPath: "data.calendar_list", UIDField: "calendar_id",
		CursorFlavor: "page_token", Implemented: true,
	},

	// ── 通讯录 / 组织架构 (list semantics need department traversal / scope;
	//    descriptors ship as documentation until the traversal driver lands) ──
	{
		Kind: "feishu_department", Domain: DomainContacts, Label: "部门",
		Endpoint: "/open-apis/contact/v3/departments", Method: "GET",
		BaseParams: map[string]string{"fetch_child": "true", "department_id_type": "open_department_id"},
		ItemPath:   "data.items", UIDField: "open_department_id",
		CursorFlavor: "page_token", Implemented: false,
	},
	{
		Kind: "feishu_user", Domain: DomainContacts, Label: "成员",
		Endpoint: "/open-apis/contact/v3/users/find_by_department", Method: "GET",
		BaseParams: map[string]string{"user_id_type": "open_id"},
		ItemPath:   "data.items", UIDField: "open_id",
		CursorFlavor: "page_token", Implemented: false,
	},

	// ── 审批 / 考勤 (require approval_code / POST-body query; documented,
	//    not yet wired) ──
	{
		Kind: "feishu_approval_instance", Domain: DomainApproval, Label: "审批实例",
		Endpoint: "/open-apis/approval/v4/instances", Method: "GET",
		BaseParams: map[string]string{}, ItemPath: "data.instance_code_list", UIDField: "",
		CursorFlavor: "page_token", Implemented: false,
	},
	{
		Kind: "feishu_attendance_record", Domain: DomainAttendance, Label: "考勤记录",
		Endpoint: "/open-apis/attendance/v1/user_tasks/query", Method: "POST",
		BaseParams: map[string]string{}, ItemPath: "data.user_task_results", UIDField: "",
		CursorFlavor: "timestamp", Implemented: false,
	},
}

// Catalog returns a copy of the full descriptor table (for the config UI, which
// lists every kind — implemented or planned — so users see the roadmap).
func Catalog() []CollectionDescriptor {
	out := make([]CollectionDescriptor, len(catalog))
	copy(out, catalog)
	return out
}

// DescriptorFor returns the descriptor for a kind, or nil when unknown.
func DescriptorFor(kind string) *CollectionDescriptor {
	for i := range catalog {
		if catalog[i].Kind == kind {
			return &catalog[i]
		}
	}
	return nil
}
