package utils

import (
	"context"
	"fmt"
	"reflect"
)

// CallFunctionByName Helper function to call a function by name from the map
func CallFunctionByName(
	ctx context.Context,
	funcMap map[string]any,
	name string,
	inputs ...any,
) ([]reflect.Value, error) {
	fn, ok := funcMap[name]
	if !ok {
		return nil, fmt.Errorf("function not found: %s", name)
	}

	// Convert inputs to []reflect.Value
	inputValues := make([]reflect.Value, len(inputs)+1)
	inputValues[0] = reflect.ValueOf(ctx)
	for i, input := range inputs {
		inputValues[i+1] = reflect.ValueOf(input)
	}

	output := reflect.ValueOf(fn).Call(inputValues)
	return output, nil
}
