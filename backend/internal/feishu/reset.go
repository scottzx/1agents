package feishu

// reset.go supports the "重置本地数据" feature: wipe the synced Feishu/Lark chat
// data in sync.db in place, keeping the file and schema intact so the running
// server keeps using the same connection.

// ClearAllData deletes every row from the sync.db tables in a single
// transaction. The file and schema are left intact.
func (s *Store) ClearAllData() error {
	tx, err := s.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"unified_messages", "unified_sync_watermarks"} {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	return tx.Commit()
}
