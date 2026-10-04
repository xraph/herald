package extension

import (
	dashboard "github.com/xraph/forge/extensions/dashboard"
)

// The dashboard finds Herald's contract contributor at runtime, so the
// shipped package never imports forge's dashboard root (which drags in its
// templ pages). This keeps the method signature checked against the real
// interface anyway.
var _ dashboard.ContractContributorAware = (*Extension)(nil)
