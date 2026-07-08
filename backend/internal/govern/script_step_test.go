package govern

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/data"
)

// volumeScript is the same nested-array aggregation as examples/.../xunji_volume.py,
// inlined so the test is self-contained.
const volumeScript = `import sys, json
for line in sys.stdin:
    line = line.strip()
    if not line: continue
    r = json.loads(line)
    p = json.loads(r.get("payload") or "{}")
    vol = 0.0; sets = 0
    for m in p.get("movements", []):
        for s in m.get("sets", []):
            if not s.get("done"): continue
            sets += 1
            vol += float(s.get("weight") or 0) * int(s.get("reps") or 0)
    print(json.dumps({"external_id": r["external_id"], "total_volume_kg": round(vol,1),
                      "total_sets": sets, "updated_at": r["updated_at"]}))
`

func TestScriptStep_PythonTransformUpsertIncremental(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	t.Setenv("ONEAGENTS_HOME", t.TempDir())
	st, err := data.OpenDefault()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db := st.SQL()
	mustExec(t, db, `CREATE TABLE silver_xunji (external_id TEXT PRIMARY KEY, payload TEXT, updated_at INTEGER)`)
	mustExec(t, db, `INSERT INTO silver_xunji VALUES
		('t1','{"movements":[{"sets":[{"weight":"30","reps":"6","done":true},{"weight":"20","reps":"10","done":true}]}]}',100)`)

	scriptPath := filepath.Join(t.TempDir(), "vol.py")
	if err := os.WriteFile(scriptPath, []byte(volumeScript), 0o644); err != nil {
		t.Fatal(err)
	}

	step := ScriptStep{
		Name: "gold_volume", Upstreams: []string{"silver_xunji"}, Output: "gold_volume",
		Script: scriptPath, InputSQL: "SELECT external_id, payload, updated_at FROM silver_xunji WHERE updated_at > :since",
		IncrCol: "updated_at", Conflict: []string{"external_id"},
		CreateSQL: `CREATE TABLE IF NOT EXISTS gold_volume (
			external_id TEXT PRIMARY KEY, total_volume_kg REAL, total_sets INTEGER, updated_at INTEGER)`,
	}

	// 30*6 + 20*10 = 380.
	if n, err := RunScriptStep(st, step); err != nil || n != 1 {
		t.Fatalf("run1 = %d/%v, want 1", n, err)
	}
	var vol float64
	var sets int
	if err := db.QueryRow(`SELECT total_volume_kg, total_sets FROM gold_volume WHERE external_id='t1'`).Scan(&vol, &sets); err != nil {
		t.Fatalf("read: %v", err)
	}
	if vol != 380 || sets != 2 {
		t.Fatalf("vol=%v sets=%d, want 380/2", vol, sets)
	}

	// Re-run: cursor gates it to zero.
	if n, err := RunScriptStep(st, step); err != nil || n != 0 {
		t.Fatalf("run2 = %d/%v, want 0", n, err)
	}

	// Update past the watermark: upsert overwrites.
	mustExec(t, db, `UPDATE silver_xunji SET payload='{"movements":[{"sets":[{"weight":"50","reps":"5","done":true}]}]}', updated_at=200 WHERE external_id='t1'`)
	if n, err := RunScriptStep(st, step); err != nil || n != 1 {
		t.Fatalf("run3 = %d/%v, want 1", n, err)
	}
	if err := db.QueryRow(`SELECT total_volume_kg FROM gold_volume WHERE external_id='t1'`).Scan(&vol); err != nil {
		t.Fatal(err)
	}
	if vol != 250 { // 50*5
		t.Fatalf("vol after update = %v, want 250", vol)
	}
}
