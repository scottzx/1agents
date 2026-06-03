use std::sync::{Arc, Mutex};
use tauri::Manager;

// Find a free TCP port dynamically
fn find_free_port() -> Option<u16> {
    std::net::TcpListener::bind("127.0.0.1:0")
        .ok()
        .and_then(|listener| listener.local_addr().ok())
        .map(|addr| addr.port())
}

fn adhoc_sign(path: &std::path::Path) {
    #[cfg(target_os = "macos")]
    {
        if path.exists() {
            println!("[tauri] Ad-hoc signing {:?}", path);
            let _ = std::process::Command::new("codesign")
                .arg("--force")
                .arg("--sign")
                .arg("-")
                .arg(path)
                .status();
        }
    }
}

#[derive(serde::Deserialize)]
struct DaemonInfo {
    listen_addr: String,
    #[allow(dead_code)]
    pid: i32,
}

// Check if the address is active (can establish a TCP connection)
fn is_address_active(addr: &str) -> bool {
    let target = if addr.starts_with(':') {
        format!("127.0.0.1{}", addr)
    } else if addr.starts_with("0.0.0.0:") {
        addr.replace("0.0.0.0:", "127.0.0.1:")
    } else {
        addr.to_string()
    };

    use std::net::ToSocketAddrs;
    if let Ok(addrs) = target.to_socket_addrs() {
        for socket_addr in addrs {
            if std::net::TcpStream::connect_timeout(&socket_addr, std::time::Duration::from_millis(200)).is_ok() {
                return true;
            }
        }
    }
    false
}

// Find if there is an active daemon running
fn get_active_daemon_addr(app: &tauri::App) -> Option<String> {
    let home_dir = app.path().home_dir().ok()?;
    let daemon_path = home_dir.join(".1agents").join("daemon.json");
    if daemon_path.exists() {
        if let Ok(content) = std::fs::read_to_string(&daemon_path) {
            if let Ok(info) = serde_json::from_str::<DaemonInfo>(&content) {
                if is_address_active(&info.listen_addr) {
                    let normalized = if info.listen_addr.starts_with(':') {
                        format!("127.0.0.1{}", info.listen_addr)
                    } else if info.listen_addr.starts_with("0.0.0.0:") {
                        info.listen_addr.replace("0.0.0.0:", "127.0.0.1:")
                    } else {
                        info.listen_addr
                    };
                    return Some(normalized);
                }
            }
        }
    }
    None
}

#[derive(Default)]
pub struct BrowserWebviewsState(pub std::sync::Mutex<std::collections::HashMap<String, tauri::webview::Webview>>);

#[tauri::command]
async fn create_browser_tab(
    app: tauri::AppHandle,
    state: tauri::State<'_, BrowserWebviewsState>,
    tab_id: String,
    url: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
) -> Result<(), String> {
    let window = app.get_window("main").ok_or("Main window not found")?;
    
    let mut map = state.0.lock().unwrap();
    if map.contains_key(&tab_id) {
        return Ok(());
    }

    let parsed_url = tauri::Url::parse(&url).map_err(|e| e.to_string())?;

    // Create the child webview using WebviewBuilder
    let webview = window.add_child(
        tauri::webview::WebviewBuilder::new(&tab_id, tauri::WebviewUrl::External(parsed_url)),
        tauri::LogicalPosition::new(x, y),
        tauri::LogicalSize::new(width, height),
    ).map_err(|e| e.to_string())?;

    map.insert(tab_id, webview);
    Ok(())
}

#[tauri::command]
async fn update_browser_tab_bounds(
    state: tauri::State<'_, BrowserWebviewsState>,
    tab_id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
) -> Result<(), String> {
    let map = state.0.lock().unwrap();
    if let Some(webview) = map.get(&tab_id) {
        webview.set_position(tauri::LogicalPosition::new(x, y)).map_err(|e| e.to_string())?;
        webview.set_size(tauri::LogicalSize::new(width, height)).map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn navigate_browser_tab(
    state: tauri::State<'_, BrowserWebviewsState>,
    tab_id: String,
    url: String,
) -> Result<(), String> {
    let map = state.0.lock().unwrap();
    if let Some(webview) = map.get(&tab_id) {
        let parsed_url = tauri::Url::parse(&url).map_err(|e| e.to_string())?;
        webview.navigate(parsed_url).map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn show_browser_tab(
    state: tauri::State<'_, BrowserWebviewsState>,
    tab_id: String,
) -> Result<(), String> {
    let map = state.0.lock().unwrap();
    for (id, webview) in map.iter() {
        if id == &tab_id {
            webview.show().map_err(|e| e.to_string())?;
        } else {
            webview.hide().map_err(|e| e.to_string())?;
        }
    }
    Ok(())
}

#[tauri::command]
async fn hide_all_browser_tabs(
    state: tauri::State<'_, BrowserWebviewsState>,
) -> Result<(), String> {
    let map = state.0.lock().unwrap();
    for webview in map.values() {
        webview.hide().map_err(|e| e.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn destroy_browser_tab(
    state: tauri::State<'_, BrowserWebviewsState>,
    tab_id: String,
) -> Result<(), String> {
    let mut map = state.0.lock().unwrap();
    map.remove(&tab_id);
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let child_process = Arc::new(Mutex::new(None));
    let child_process_clone = Arc::clone(&child_process);

    let app = tauri::Builder::default()
        .manage(BrowserWebviewsState::default())
        .invoke_handler(tauri::generate_handler![
            create_browser_tab,
            update_browser_tab_bounds,
            navigate_browser_tab,
            show_browser_tab,
            hide_all_browser_tabs,
            destroy_browser_tab
        ])
        .setup(move |app| {
            if cfg!(debug_assertions) {
                let _ = app.handle().plugin(
                    tauri_plugin_log::Builder::default()
                        .level(log::LevelFilter::Info)
                        .build(),
                );
            }

            let url;
            let mut spawned = false;

            if let Some(active_addr) = get_active_daemon_addr(app) {
                println!("[tauri] Found active daemon running at {}. Reusing it.", active_addr);
                url = if active_addr.starts_with("http://") || active_addr.starts_with("https://") {
                    active_addr
                } else {
                    format!("http://{}", active_addr)
                };
            } else {
                // Find an available port
                let port = find_free_port().unwrap_or(38080);
                let listen_addr = format!("127.0.0.1:{}", port);
                url = format!("http://{}", listen_addr);

                // Resolve the path to the tauri resources directory
                let resource_dir = app.path().resource_dir().expect("failed to get resource directory");
                
                // Re-sign binaries on macOS to prevent SIGKILL due to macOS signature caching on file replacement
                let bin_dir = resource_dir.join("resources").join("bin");
                let node_dir = resource_dir.join("resources").join("runtime").join("node").join("bin");
                adhoc_sign(&bin_dir.join("1agents"));
                adhoc_sign(&bin_dir.join("ttyd"));
                adhoc_sign(&bin_dir.join("cc-connect"));
                adhoc_sign(&node_dir.join("node"));

                // Resolve 1agents backend path inside resources/bin/
                let mut daemon_path = bin_dir.join("1agents");
                if cfg!(target_os = "windows") {
                    daemon_path.set_extension("exe");
                }

                println!("[tauri] Spawning Go daemon from path: {:?}", daemon_path);
                println!("[tauri] Passing -resources-dir: {:?}", resource_dir);
                println!("[tauri] Listen address: {}", listen_addr);

                // Spawn the Go daemon child process
                let child = std::process::Command::new(&daemon_path)
                    .arg("-listen")
                    .arg(&listen_addr)
                    .arg("-desktop")
                    .arg("-resources-dir")
                    .arg(&resource_dir)
                    .stdout(std::process::Stdio::inherit())
                    .stderr(std::process::Stdio::inherit())
                    .spawn();

                match child {
                    Ok(c) => {
                        *child_process_clone.lock().unwrap() = Some(c);
                        println!("[tauri] Go daemon spawned successfully.");
                        spawned = true;
                    }
                    Err(e) => {
                        eprintln!("[tauri] ERROR: Failed to spawn Go daemon: {:?}", e);
                    }
                }

                if spawned {
                    // Wait for the Go backend HTTP server to bind (max 5 seconds)
                    println!("[tauri] Waiting for Go daemon to bind to port {}...", port);
                    let mut port_ready = false;
                    for _ in 0..50 {
                        if std::net::TcpStream::connect(format!("127.0.0.1:{}", port)).is_ok() {
                            port_ready = true;
                            break;
                        }
                        std::thread::sleep(std::time::Duration::from_millis(100));
                    }
                    if port_ready {
                        println!("[tauri] Go daemon is ready and listening on port {}.", port);
                    } else {
                        eprintln!("[tauri] WARNING: Go daemon port {} did not become ready in time", port);
                    }
                }
            }

            // Navigate the main window webview to the Go HTTP server
            if let Some(window) = app.get_webview_window("main") {
                println!("[tauri] Navigating webview window to: {}", url);
                
                #[cfg(debug_assertions)]
                {
                    let _ = window.open_devtools();
                }

                if let Ok(parsed_url) = tauri::Url::parse(&url) {
                    let _ = window.navigate(parsed_url);
                }
            } else {
                eprintln!("[tauri] ERROR: Could not find main webview window.");
            }

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    app.run(move |_app_handle, event| {
        if let tauri::RunEvent::ExitRequested { .. } = event {
            // Terminate Go daemon child process when Tauri app exits
            if let Some(mut child) = child_process.lock().unwrap().take() {
                println!("[tauri] Terminating Go daemon...");
                let _ = child.kill();
                let _ = child.wait();
            }
        }
    });
}
