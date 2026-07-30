use std::{fs, path::PathBuf};

#[test]
fn desktop_webview_allows_html_drag_and_drop() {
    let config_path = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tauri.conf.json");
    let config: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(config_path).expect("read tauri.conf.json"))
            .expect("parse tauri.conf.json");

    assert_eq!(
        config["app"]["windows"][0]["dragDropEnabled"].as_bool(),
        Some(false),
        "Tauri's native drag/drop handler must be disabled so frontend HTML drag events reach WebView2"
    );
}
