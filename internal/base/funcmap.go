package base

import (
	"errors"
	"fmt"
	"reflect"
)

// funcMap Map of function names to functions
var funcMap = map[string]interface{}{
	"ping": handlePing,
}

// callFunctionByName Helper function to call a function by its name from the map
func callFunctionByName(name string, input interface{}) ([]reflect.Value, error) {
	fn, ok := funcMap[name]
	if !ok {
		return nil, errors.New(fmt.Sprintf("function not found: %s", name))
	}
	inputValue := []reflect.Value{reflect.ValueOf(input)}
	output := reflect.ValueOf(fn).Call(inputValue)
	return output, nil
}
