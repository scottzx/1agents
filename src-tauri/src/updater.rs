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
    use tauri_plugin_updater::UpdaterExt;

    let current = app.package_info().version.to_string();

    let check_result = app
        .updater_builder()
        .on_before_exit(|| {
            // Let the dev see what happened; real exit is handled by
            // tauri-plugin-process's relaunch() in JS.
        })
        .build()
        .map_err(|e| format!("updater builder failed: {}", e))?
        .check()
        .await
        .map_err(|e| format!("update check failed: {}", e))?;

    match check_result {
        Some(update) => Ok(DesktopUpdateInfo {
            available: true,
            current,
            latest: update.version.clone(),
            notes: update.body.clone(),
        }),
        None => Ok(DesktopUpdateInfo {
            available: false,
            current,
            latest: current.clone(),
            notes: None,
        }),
    }
}
