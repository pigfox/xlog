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
