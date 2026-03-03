package main

import (
	"errors"
	"os"

	"github.com/pigfox/xlog"
	"github.com/pigfox/xlog/samplepkg"
)

func main() {
	xlog.Init(os.Stdout)

	// All combinations from main:
	xlog.Info("info from main")
	xlog.Error(errors.New("plain error from main"))
	xlog.Error2("db", errors.New("prefixed error from main"))
	xlog.Error(nil)
	xlog.Error2("ignored", nil)

	// All combinations from another package:
	samplepkg.Run()
}
