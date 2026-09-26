//go:build !linux

package costtest

import "time"

// onThreadClock is the wall clock where there is no thread clock to ask, which
// measures the same curve with more of the machine's business in it. See the
// Linux version.
func onThreadClock(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Since(start)
}
