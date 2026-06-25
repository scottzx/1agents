// Package opensource is the 开源项目吸收机制 (#138): the Inbox 收口层 (#60)
// downstream consumer that turns "方向一致、值得吸收" 的开源项目 into a reviewed
// 吸收提案 (absorb proposal).
//
// Pipeline (minimal, 无 LLM, 无向量库):
//
//	Candidate (仓库元信息 + license)
//	  → license 合规判定 (SPDX 白名单：MIT/Apache 等可合并; 弱 copyleft 仅借鉴;
//	    强 copyleft / 未知 → 不合并)
//	  → 评审打分 (契合度 fit + 质量 quality 启发式)
//	  → Proposal (决策: merge / borrow / ignore)
//	  → 落 Inbox (复用 #188 absorb 的「转化成提案再人审」思路, 不直接落地代码)。
//
// 外部抓取 (GitHub 搜索等) 做成 Fetcher 接口 + 可注入实现 + 占位实现, 保证可单测,
// 不强依赖实时网络 (= inbox.InboxSource 的 seam 思路)。
package opensource

import "strings"

// License is the merge-ability classification of an SPDX license id.
type License int

const (
	// LicenseUnknown is an empty/unrecognized license — never auto-mergeable.
	LicenseUnknown License = iota
	// LicenseMergeable is a permissive license (MIT/Apache-2.0/BSD/ISC) whose
	// terms allow vendoring the source straight into our own MIT-style project,
	// keeping attribution.
	LicenseMergeable
	// LicenseBorrowOnly is a weak-copyleft license (MPL-2.0/LGPL): the *ideas*
	// can be borrowed but the source must not be vendored wholesale.
	LicenseBorrowOnly
	// LicenseIncompatible is a strong-copyleft / proprietary license (GPL/AGPL):
	// neither merge nor file-level vendoring is allowed.
	LicenseIncompatible
)

// mergeableLicenses is the permissive SPDX whitelist — these may be merged.
var mergeableLicenses = map[string]bool{
	"MIT":          true,
	"APACHE-2.0":   true,
	"BSD-2-CLAUSE": true,
	"BSD-3-CLAUSE": true,
	"ISC":          true,
	"0BSD":         true,
	"UNLICENSE":    true,
}

// borrowOnlyLicenses are weak-copyleft: borrow ideas, do not vendor source.
var borrowOnlyLicenses = map[string]bool{
	"MPL-2.0":  true,
	"LGPL-2.1": true,
	"LGPL-3.0": true,
	"EPL-2.0":  true,
}

// ClassifyLicense maps a raw license string (SPDX id or loose name) onto a
// merge-ability class. Matching is case-insensitive and tolerates the common
// "-only"/"-or-later" suffixes and a few non-SPDX aliases ("Apache 2.0", "BSD").
func ClassifyLicense(raw string) License {
	id := normalizeLicenseID(raw)
	if id == "" {
		return LicenseUnknown
	}
	if mergeableLicenses[id] {
		return LicenseMergeable
	}
	if borrowOnlyLicenses[id] {
		return LicenseBorrowOnly
	}
	if strings.HasPrefix(id, "GPL-") || strings.HasPrefix(id, "AGPL-") {
		return LicenseIncompatible
	}
	// Recognized-as-restrictive families collapse here; everything else is unknown
	// so it is never silently merged.
	if id == "PROPRIETARY" || id == "NONE" {
		return LicenseIncompatible
	}
	return LicenseUnknown
}

// normalizeLicenseID upper-cases, trims, and folds common variants onto a
// canonical SPDX-ish id used by the lookup maps.
func normalizeLicenseID(raw string) string {
	id := strings.ToUpper(strings.TrimSpace(raw))
	if id == "" {
		return ""
	}
	// Drop SPDX modifier suffixes that don't change merge-ability.
	id = strings.TrimSuffix(id, "-ONLY")
	id = strings.TrimSuffix(id, "-OR-LATER")
	// Loose aliases people actually type.
	switch id {
	case "APACHE", "APACHE 2.0", "APACHE-2", "APACHE2", "APACHE LICENSE 2.0":
		return "APACHE-2.0"
	case "MIT LICENSE":
		return "MIT"
	case "BSD":
		return "BSD-3-CLAUSE"
	case "GPL", "GPLV2", "GPLV3":
		return "GPL-3.0"
	case "AGPL", "AGPLV3":
		return "AGPL-3.0"
	}
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

// CanMerge reports whether a license class permits source-level merge.
func (l License) CanMerge() bool { return l == LicenseMergeable }

func (l License) String() string {
	switch l {
	case LicenseMergeable:
		return "mergeable"
	case LicenseBorrowOnly:
		return "borrow-only"
	case LicenseIncompatible:
		return "incompatible"
	default:
		return "unknown"
	}
}
