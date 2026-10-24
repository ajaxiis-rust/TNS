package main

import (
	"flag"
	"fmt"

	"tns65/internal/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and build date")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Version: %s\nBuildDate: %s\n", version.Version, version.BuildDate)
	}
}
