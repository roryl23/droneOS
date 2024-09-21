package base

import (
	"droneOS/internal/protocol"
	"errors"
	"fmt"
	"reflect"
)

// funcMap Map of function names to functions
var funcMap = map[string]interface{}{
	"ping": protocol.Ping,
}

// callFunctionByName Helper function to call a function by its name from the map
func callFunctionByName(msg protocol.Message) ([]reflect.Value, error) {
	fn, ok := funcMap[msg.Cmd]
	if !ok {
		return nil, errors.New(fmt.Sprintf("function not found: %s", msg.Cmd))
	}
	inputValue := []reflect.Value{reflect.ValueOf(msg)}
	output := reflect.ValueOf(fn).Call(inputValue)
	return output, nil
}
