// Command linnkit maps Scala scales onto a LinnStrument. Without arguments it
// opens the dashboard.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) == 1 {
		os.Exit(runTUI(nil, os.Stderr))
	}
	switch os.Args[1] {
	case "tui":
		os.Exit(runTUI(os.Args[2:], os.Stderr))
	case "describe":
		os.Exit(runDescribe(os.Args[2:], os.Stdout, os.Stderr))
	case "grid":
		os.Exit(runGrid(os.Args[2:], os.Stdout, os.Stderr))
	case "device":
		os.Exit(runDevice(os.Args[2:], os.Stdout, os.Stderr))
	case "send":
		os.Exit(runSend(os.Args[2:], os.Stdout, os.Stderr))
	case "export":
		os.Exit(runExport(os.Args[2:], os.Stdout, os.Stderr))
	case "folders":
		os.Exit(runFolders(os.Args[2:], os.Stdout, os.Stderr))
	}

	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage:")
		fmt.Fprintln(os.Stderr, "  linnkit                     the dashboard (also: linnkit tui [--scales dirs] [--port name])")
		fmt.Fprintln(os.Stderr, "  linnkit describe [--matrix] [--tol cents] FILE...")
		fmt.Fprintln(os.Stderr, "  linnkit grid [--offset n] [--low n] [--root n] [--scheme ji|names|mos|root] [--color] FILE")
		fmt.Fprintln(os.Stderr, "  linnkit device ports | read [NUM...] | backup FILE | restore FILE")
		fmt.Fprintln(os.Stderr, "  linnkit send --slot 0|1|2 [--layout] [--configure] [grid flags] FILE")
		fmt.Fprintln(os.Stderr, "  linnkit export [-n] [--root n] [--dir DIR] FILE...   copy to Madrona Labs Scales/linnkit with a .kbm")
		fmt.Fprintln(os.Stderr, "  linnkit folders [add|remove DIR]   extra scale folders (app data: ~/.config/linnkit)")
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
