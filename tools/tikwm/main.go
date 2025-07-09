package main

import (
	"fmt"
	"os"

	"github.com/perpetuallyhorni/tikwm/tools/tikwm/cmd"
)

var version string = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
