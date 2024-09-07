package droneOS

// github.com/thinkski/go-v4l2
import (
	"droneOS/internal/gpio"
	"math"
	"runtime"
	"runtime/debug"
)

func main() {
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(math.MaxInt64)

	gpio.Init()

	runtime.GC()
}
