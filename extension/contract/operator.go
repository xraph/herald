package contract

import (
	"strings"

	"github.com/xraph/forge/extensions/dashboard/contract"
)

// actorFrom is the operator a command's audit event names: the session's
// subject, trimmed, or "" when there is none. An unattributed write shows no
// actor rather than a wrong one. Only commands call it.
func actorFrom(p contract.Principal) string {
	if p.User == nil {
		return ""
	}
	return strings.TrimSpace(p.User.Subject)
}
