package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cmd/dev/pi_runner is not yet ported to Alpine/OpenRC remote deployment")
	os.Exit(1)
}
