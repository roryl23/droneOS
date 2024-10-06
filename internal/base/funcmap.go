package base

import (
	xboxone "droneOS/internal/input/joystick"
	"droneOS/internal/protocol"
)

// BaseFuncMap Map of function names to functions
var BaseFuncMap = map[string]interface{}{
	"ping":     protocol.Ping,
	"xbox_one": xboxone.Main,
}
