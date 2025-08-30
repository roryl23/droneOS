package utils

import (
	"errors"
	"fmt"
	"reflect"
)

// CallFunctionByName Helper function to call a function by name from the map
func CallFunctionByName(funcMap map[string]interface{}, name string, inputs ...interface{}) ([]reflect.Value, error) {
	fn, ok := funcMap[name]
	if !ok {
		return nil, errors.New(fmt.Sprintf("function not found: %s", name))
	}

	// Convert inputs to []reflect.Value
	inputValues := make([]reflect.Value, len(inputs))
	for i, input := range inputs {
		inputValues[i] = reflect.ValueOf(input)
	}

	output := reflect.ValueOf(fn).Call(inputValues)
	return output, nil
}
