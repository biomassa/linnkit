// Command linnkit maps Scala scales onto a LinnStrument.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("linnkit", version)
		return
	}

	fmt.Fprintln(os.Stderr, "linnkit: nothing to run yet (see PLAN.md)")
	os.Exit(2)
}
