package meta

import (
	"database/sql"
	"time"
)

// feishuOfficialTenant is Feishu/Lark's official tenant_key. Degree-2 members
// carrying it are Feishu official operations staff (verified across multiple
// groups). It is seeded — not hardcoded into display logic — so the org name now
// flows through the companies map like any other org. Kept here as the single
// seed value for SeedFeishuOfficial.
const feishuOfficialTenant = "736588c9260f175d"

// Company owns one org's base metadata. UnifiedID is reserved for a future
// business id (统一社会信用代码 etc.); the live linkage key is the tenant_key,
// recorded separately in company_tenants.
type Company struct {
	ID        string    `json:"id"`
	FullName  string    `json:"fullName"`
	ShortName string    `json:"shortName"`
	UnifiedID string    `json:"unifiedId"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CompanyTenant is one company_tenants × companies join row: a mapped tenant_key
// plus the company names the frontend uses to label a contact's channel org.
type CompanyTenant struct {
	TenantKey string `json:"tenantKey"`
	CompanyID string `json:"companyId"`
	FullName  string `json:"fullName"`
	ShortName string `json:"shortName"`
}

// CompanyStore manages the 公司基础信息表 + tenant_key linkage (meta.db v19).
type CompanyStore struct {
	db *DB
}

// NewCompanyStore returns a CompanyStore over db.
func NewCompanyStore(db *DB) *CompanyStore { return &CompanyStore{db: db} }

const companyCols = `id, full_name, short_name, unified_id, note, created_at, updated_at`

func scanCompany(r rowScanner) (Company, error) {
	var c Company
	var createdAt, updatedAt string
	if err := r.Scan(&c.ID, &c.FullName, &c.ShortName, &c.UnifiedID, &c.Note, &createdAt, &updatedAt); err != nil {
		return Company{}, err
	}
	c.CreatedAt = strToTime(createdAt)
	c.UpdatedAt = strToTime(updatedAt)
	return c, nil
}

// ListCompanies returns all companies ordered by short name.
func (s *CompanyStore) ListCompanies() ([]Company, error) {
	rows, err := s.db.sql.Query(`SELECT ` + companyCols + ` FROM companies ORDER BY short_name, full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Company{}
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *CompanyStore) getCompany(id string) (Company, error) {
	row := s.db.sql.QueryRow(`SELECT `+companyCols+` FROM companies WHERE id = ?`, id)
	return scanCompany(row)
}

// CreateCompany inserts a new company with a fresh id.
func (s *CompanyStore) CreateCompany(fullName, shortName, unifiedID, note string) (Company, error) {
	now := time.Now().UTC()
	c := Company{
		ID:        newID(),
		FullName:  fullName,
		ShortName: shortName,
		UnifiedID: unifiedID,
		Note:      note,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.UpsertCompany(c); err != nil {
		return Company{}, err
	}
	return s.getCompany(c.ID)
}

// UpsertCompany inserts or replaces a company by id. CreatedAt is preserved on
// update (mirrors DigestStore.UpsertTemplate).
func (s *CompanyStore) UpsertCompany(c Company) error {
	now := time.Now().UTC()
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := s.db.sql.Exec(`INSERT INTO companies (`+companyCols+`)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            full_name = excluded.full_name, short_name = excluded.short_name,
            unified_id = excluded.unified_id, note = excluded.note,
            updated_at = excluded.updated_at`,
		c.ID, c.FullName, c.ShortName, c.UnifiedID, c.Note,
		timeToStr(c.CreatedAt), timeToStr(c.UpdatedAt))
	return err
}

// MapTenant binds a tenant_key to a company (INSERT OR REPLACE — a tenant_key
// maps to exactly one company, re-mapping moves it).
func (s *CompanyStore) MapTenant(tenantKey, companyID string) error {
	_, err := s.db.sql.Exec(`INSERT OR REPLACE INTO company_tenants (tenant_key, company_id, created_at)
        VALUES (?, ?, ?)`, tenantKey, companyID, timeToStr(time.Now().UTC()))
	return err
}

// UnmapTenant removes a tenant_key→company mapping.
func (s *CompanyStore) UnmapTenant(tenantKey string) error {
	_, err := s.db.sql.Exec(`DELETE FROM company_tenants WHERE tenant_key = ?`, tenantKey)
	return err
}

// TenantCompanyMap joins company_tenants × companies → every mapped tenant_key
// with its company names. The frontend builds a tenantKey→shortName map from it.
func (s *CompanyStore) TenantCompanyMap() ([]CompanyTenant, error) {
	rows, err := s.db.sql.Query(`SELECT ct.tenant_key, ct.company_id, c.full_name, c.short_name
        FROM company_tenants ct JOIN companies c ON c.id = ct.company_id
        ORDER BY c.short_name, ct.tenant_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CompanyTenant{}
	for rows.Next() {
		var ct CompanyTenant
		if err := rows.Scan(&ct.TenantKey, &ct.CompanyID, &ct.FullName, &ct.ShortName); err != nil {
			return nil, err
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}

// SeedFeishuOfficial ensures a 飞书官方 company exists and that the Feishu
// official tenant_key maps to it. Idempotent and insert-only: if the tenant_key
// is already mapped (to any company) it leaves the mapping untouched, so a user
// re-pointing it elsewhere is preserved. Replaces the old hardcoded constant +
// special-case in the frontend's orgLabel.
func (s *CompanyStore) SeedFeishuOfficial() error {
	// Already mapped? Leave it — insert-only.
	var existing string
	err := s.db.sql.QueryRow(`SELECT company_id FROM company_tenants WHERE tenant_key = ?`,
		feishuOfficialTenant).Scan(&existing)
	if err == nil && existing != "" {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	// Reuse an existing 飞书官方 company if one is already there; else create it.
	var companyID string
	if cerr := s.db.sql.QueryRow(`SELECT id FROM companies WHERE short_name = '飞书官方' LIMIT 1`).Scan(&companyID); cerr == sql.ErrNoRows {
		c, cerr := s.CreateCompany("飞书官方", "飞书官方", "", "")
		if cerr != nil {
			return cerr
		}
		companyID = c.ID
	} else if cerr != nil {
		return cerr
	}
	return s.MapTenant(feishuOfficialTenant, companyID)
}
