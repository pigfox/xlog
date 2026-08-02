package xlog

// callerSkip is the runtime.Callers skip count used to find the caller of an
// exported entry point. Skip 0: runtime.Callers, 1: callerFileLine, 2: callerAttr,
// 3: emit — so the walk starts at frame 4. Remaining xlog frames are dropped by
// name in firstNonXlogFrame, so wrapper methods on Logger resolve to the same
// caller as the package-level functions.
const callerSkip = 4
