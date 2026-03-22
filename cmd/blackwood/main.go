package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "", "path to the configuration file")
	watchDir := flag.String("watch-dir", "", "directory to watch for new notes files")
	flag.Parse()

	if *configPath == "" || *watchDir == "" {
		fmt.Fprintln(os.Stderr, "both --config and --watch-dir are required")
		flag.Usage()
		os.Exit(1)
	}

	fmt.Printf("config: %s\nwatch-dir: %s\n", *configPath, *watchDir)
}
