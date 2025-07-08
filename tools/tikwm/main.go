package main

import (
	"fmt"
	"os"

	"github.com/perpetuallyhorni/tikwm/tools/tikwm/cmd"
)

func main() {
	// Execute the command-line interface.
	if err := cmd.Execute(); err != nil {
		// If an error occurs during command execution, print the error to stderr.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// Exit the program with a non-zero exit code to indicate failure.
		os.Exit(1)
	}
}
