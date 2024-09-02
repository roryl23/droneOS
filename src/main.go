package main

// github.com/warthog618/go-gpiocdev
// github.com/thinkski/go-v4l2
import (
	"math"
	"runtime"
	"runtime/debug"
)

func main() {
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	runtime.GC()
}
