package domainownership

// RegisterKernelLedger brings the pre-existing kernel tables and write APIs
// under ownership management without renaming them (§7.1: 已有核心表通过所
// 有权清单纳管). New kernel tables use the kernel_ prefix instead (e.g.
// kernel_access_denials) and register through RegisterTable.
//
// The list mirrors the DDL in internal/meta, internal/commandbus and
// internal/appregistry; the architecture test TestKernelLedgerCoversAllDDL
// fails when a kernel package adds a table that is neither ledgered nor
// kernel_-prefixed.
func RegisterKernelLedger(r *Registry) error {
	if err := r.RegisterNamespaceOwner(NamespaceKernel, NamespaceKernel); err != nil {
		return err
	}
	// Reserved application namespaces are claimed by their architecture
	// owner up front so no other module can preempt them (§7.1). The
	// enterprise namespace stays unregistered until a capability passes the
	// promotion gate (§6.1).
	for ns, owner := range map[string]string{
		NamespacePresales: NamespacePresales,
		NamespaceCommerce: NamespaceCommerce,
	} {
		if err := r.RegisterNamespaceOwner(ns, owner); err != nil {
			return err
		}
	}

	// ── pre-existing kernel tables (meta.db) ────────────────────────────────
	legacyTables := []string{
		// workspace / project / task kernel (internal/meta)
		"projects", "project_items", "task_deps", "replies", "milestones",
		// execution & audit kernel (internal/meta)
		"sessions", "agent_turns", "task_runs", "project_events",
		// WorkCase coordination object (internal/meta, schema v30)
		"work_cases", "work_case_links",
		// feature catalog (internal/meta)
		"feature_nodes", "feature_item_links", "feature_catalog_versions",
		"feature_catalog_restore_requests",
		// inbox / digest / contacts (internal/meta)
		"inbox_items", "digest_templates", "digest_bindings",
		"contacts", "contact_channels", "companies", "company_tenants",
		// channel & source configuration (internal/meta)
		"channel_modules", "feishu_tracked_chats", "feishu_sync_config",
		"feishu_group_members", "source_accounts", "source_collection_config",
		// Command infrastructure (internal/commandbus)
		"command_idempotency", "command_executions",
		// Outbox Event delivery layer (internal/outbox): the dispatcher only
		// ever mutates delivery metadata here (§7.1).
		"outbox_events", "outbox_receipts",
		// app & shell registry state (internal/appregistry)
		"app_state", "shell_state", "shell_user_preference", "product_profile",
	}
	for _, t := range legacyTables {
		if err := r.RegisterLegacyTable(NamespaceKernel, t); err != nil {
			return err
		}
	}
	// New-style kernel tables created by this package.
	if err := r.RegisterTable(NamespaceKernel, "kernel_access_denials"); err != nil {
		return err
	}
	for _, table := range []string{"kernel_execution_jobs", "kernel_execution_triggers"} {
		if err := r.RegisterTable(NamespaceKernel, table); err != nil {
			return err
		}
	}

	// ── kernel write APIs (command contracts registered on the bus) ────────
	// internal/meta/workcase_commands.go owns the WorkCase mutation
	// contracts; commandbus rejects duplicate contract registration, and the
	// ledger records which namespace owns each contract.
	for _, c := range []string{
		"workcase.create", "workcase.update", "workcase.transition",
		"workcase.delete", "workcase.link", "workcase.unlink", "workcase.set_phase",
	} {
		if err := r.RegisterWriteAPI(NamespaceKernel, c); err != nil {
			return err
		}
	}
	return nil
}
