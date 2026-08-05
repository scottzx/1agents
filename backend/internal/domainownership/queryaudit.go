package domainownership

import (
	"github.com/scottzx/1Agents/backend/internal/domainref"
)

// WireQueryDenialAudit installs the domainref denial hook so every
// permission_denied produced by a Query provider is audited (§13.3: 权限拒
// 绝被审计). The Query path stays read-only and stdlib-pure; auditing is a
// kernel concern hooked in at the composition root.
func WireQueryDenialAudit() {
	domainref.SetDenialHook(func(req domainref.QueryRequest, err error) {
		reason := ""
		if err != nil {
			reason = err.Error()
		}
		RecordDenial(Denial{
			Actor:           req.Actor,
			Action:          ActionQueryPermission,
			TargetNamespace: req.Ref.Namespace,
			Target:          req.Ref.String(),
			Code:            string(domainref.CodePermissionDenied),
			Reason:          reason,
			CorrelationID:   req.CorrelationID,
		})
	})
}
