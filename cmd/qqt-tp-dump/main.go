package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"qqtang/internal/tooling/windebug"
)

func main() {
	executable := flag.String("exe", "", "executable to debug")
	workingDir := flag.String("workdir", "", "working directory")
	dumpDir := flag.String("dump", "", "directory for module dumps")
	timeout := flag.Duration("timeout", 25*time.Second, "how long to observe the process")
	breakRVA := flag.Uint("break-rva", 0, "optional RVA to break at (hex or decimal)")
	hwBreakRVA := flag.Uint("hw-break-rva", 0, "optional RVA for a hardware execution breakpoint")
	flag.Parse()
	if *executable == "" || *dumpDir == "" {
		fmt.Fprintln(os.Stderr, "-exe and -dump are required")
		os.Exit(2)
	}
	err := windebug.Run(windebug.Options{
		Executable:       *executable,
		WorkingDir:       *workingDir,
		DumpDirectory:    *dumpDir,
		Timeout:          *timeout,
		BreakRVA:         uint32(*breakRVA),
		HardwareBreakRVA: uint32(*hwBreakRVA),
		Output:           os.Stdout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "debug run:", err)
		os.Exit(1)
	}
}
