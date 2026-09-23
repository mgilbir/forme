//go:build linux

package paragraph

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// onThreadClock runs f with the calling goroutine held to one thread and
// returns the processor time that thread spent in it.
//
// It is what the cost guards time, rather than the clock on the wall. A ratio
// of two wall-clock times is a ratio of two samples of how busy the machine
// was, and a machine running a test suite is busy: with four packages being
// tested beside this one the same linear walk read as a factor of anything from
// three to twelve. Time the thread did not run is not counted here, so a curve
// is a curve however many other things wanted the processor.
//
// The clock is CLOCK_THREAD_CPUTIME_ID, which the kernel keeps to the
// nanosecond. getrusage's per-thread times answer the same question to the
// scheduler's tick, which read a few hundred microseconds of work as none.
func onThreadClock(f func()) time.Duration {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	before := threadTime()
	f()
	return threadTime() - before
}

// clockThreadCPUTime is CLOCK_THREAD_CPUTIME_ID, which the syscall package
// does not name.
const clockThreadCPUTime = 3

func threadTime() time.Duration {
	var ts syscall.Timespec
	if _, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockThreadCPUTime,
		uintptr(unsafe.Pointer(&ts)), 0); errno != 0 {
		panic(errno)
	}
	return time.Duration(ts.Nano())
}
