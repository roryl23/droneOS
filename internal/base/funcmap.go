package base

import (
	"droneOS/internal/protocol"
)

// BaseFuncMap Map of function names to functions
var BaseFuncMap = map[string]interface{}{
	"ping": protocol.Ping,
}
