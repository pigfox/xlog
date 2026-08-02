// Package samplepkg exercises xlog from outside the xlog package, so the
// caller attribute it records resolves to this file rather than to xlog itself.
package samplepkg

import (
	"errors"

	"github.com/pigfox/xlog"
)

// Run emits all xlog call combinations from a non-main package.
func Run() {
	xlog.Info("info from samplepkg")

	xlog.Error(errors.New("plain error from samplepkg"))
	xlog.Error2("samplepkg", errors.New("prefixed error from samplepkg"))

	// No output (nil error).
	xlog.Error(nil)
	xlog.Error2("ignored", nil)
}
