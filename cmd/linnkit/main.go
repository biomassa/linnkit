// Command linnkit maps Scala scales onto a LinnStrument.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "describe":
			os.Exit(runDescribe(os.Args[2:], os.Stdout, os.Stderr))
		case "grid":
			os.Exit(runGrid(os.Args[2:], os.Stdout, os.Stderr))
		}
	}

	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage:")
		fmt.Fprintln(os.Stderr, "  linnkit describe [--matrix] [--tol cents] FILE...")
		fmt.Fprintln(os.Stderr, "  linnkit grid [--offset n] [--low n] [--root n] [--scheme ji|mos|root] [--color] FILE")
		fmt.Fprintln(os.Stderr, "  linnkit --version")
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("linnkit", version)
		return
	}
	flag.Usage()
	os.Exit(2)
}
