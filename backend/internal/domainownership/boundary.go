package domainownership

import "fmt"

// Repository exposure boundary (§3.4, §4.2): each domain's repository (the
// object that owns SQL access to the domain's tables) is private to the
// owning domain. Other domains must not hold or call it; they interact
// through the owner's Command/Query contracts.
//
// Enforcement is two-layered:
//
//  1. Statically — the architecture gate rejects any import of an app's
//     repository package from outside the app (RuleForeignRepoImport).
//  2. At runtime — repositories that cross a shared seam (e.g. a test rig
//     or a future in-process bus handing stores around) call
//     CheckRepositoryAccess, which denies and audits foreign callers.

// RegisterRepository records that namespace ns owns a repository named
// name. Names are unique within a namespace; re-registration by the same
// namespace is idempotent.
func (r *Registry) RegisterRepository(ns, name string) error {
	if !isIdent(ns) {
		return NewError(CodeInvalidDeclaration, "namespace %q must be a lowercase identifier", ns)
	}
	if !isIdent(name) {
		return NewError(CodeInvalidDeclaration, "repository name %q must be a lowercase identifier", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nsOwner[ns]; !ok {
		return NewError(CodeUnknownNamespace, "namespace %q is not registered", ns)
	}
	key := ns + "/" + name
	if cur, dup := r.repoOwner[key]; dup {
		if cur == ns {
			return nil
		}
		return NewError(CodeDuplicateOwner, "repository %q already owned by %q", key, cur)
	}
	r.repoOwner[key] = ns
	return nil
}

// CheckRepositoryAccess decides whether callerNS may directly use
// repository ns/name. Only the owning namespace may; everyone else gets a
// structured denial that is also audited. The repository name is recorded
// on the denial so audits can trace which seam was probed.
func (r *Registry) CheckRepositoryAccess(callerNS, ns, name string) error {
	if callerNS == ns {
		return nil
	}
	err := &Error{
		Code:            CodeRepositoryAccess,
		Message:         fmt.Sprintf("namespace %q may not access repository %q of namespace %q; use the owner's Command/Query contracts", callerNS, name, ns),
		CallerNamespace: callerNS,
		TargetNamespace: ns,
		Target:          name,
	}
	RecordDenial(Denial{
		CallerNamespace: callerNS,
		Action:          ActionRepository,
		TargetNamespace: ns,
		Target:          name,
		Code:            string(err.Code),
		Reason:          err.Message,
	})
	return err
}
