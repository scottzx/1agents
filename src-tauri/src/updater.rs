// OTA update bridge — thin wrapper around tauri-plugin-updater that
// the frontend can call via @tauri-apps/api invocation.
//
// V1 intentionally leaves pubkey blank (no ed25519 signature verification)
// because the provisioning story for code-signing certificates is TBD.
// See docs/ota-architecture.md §7.

use serde::Serialize;

/// What the frontend receives from `check_desktop_update()`.
#[derive(Debug, Serialize)]
pub struct DesktopUpdateInfo {
    pub available: bool,
    pub current: String,
    pub latest: String,
    pub notes: Option<String>,
}

/// Check whether a newer Tauri bundle is available on the configured
/// updater endpoints. Returns `{ available: false }` when already
/// current or when the check is skipped (first-run / dev build).
#[tauri::command]
pub async fn check_desktop_update(app: tauri::AppHandle) -> Result<DesktopUpdateInfo, String> {
    let current = app.package_info().version.to_string();

    // ── Desktop OTA temporarily disabled — see GitHub issue #55 ───────────────
    // Desktop installers are now distributed privately via Aliyun Drive (PR #53)
    // instead of the public GitHub Release, so the updater endpoint configured in
    // tauri.conf.json (releases/latest/download/desktop-{{target}}-{{arch}}.json)
    // no longer exists and would 404. Short-circuit to "no update" here because
    // tauri.conf.json is strict JSON and cannot carry a disabling comment.
    // To re-enable: remove this short-circuit, restore the block below, and point
    // plugins.updater.endpoints at a working private update source.
    Ok(DesktopUpdateInfo {
        available: false,
        current: current.clone(),
        latest: current,
        notes: None,
    })

    // --- original implementation, disabled pending a private update source ---
    // use tauri_plugin_updater::UpdaterExt;
    //
    // let check_result = app
    //     .updater_builder()
    //     .on_before_exit(|| {
    //         // Let the dev see what happened; real exit is handled by
    //         // tauri-plugin-process's relaunch() in JS.
    //     })
    //     .build()
    //     .map_err(|e| format!("updater builder failed: {}", e))?
    //     .check()
    //     .await
    //     .map_err(|e| format!("update check failed: {}", e))?;
    //
    // match check_result {
    //     Some(update) => Ok(DesktopUpdateInfo {
    //         available: true,
    //         current,
    //         latest: update.version.clone(),
    //         notes: update.body.clone(),
    //     }),
    //     None => Ok(DesktopUpdateInfo {
    //         available: false,
    //         current,
    //         latest: current.clone(),
    //         notes: None,
    //     }),
    // }
}
