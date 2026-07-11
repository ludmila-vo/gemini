package main

import (
	"fmt"
	"runtime/debug"
)

func printVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("No build information available")
		return
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		fmt.Println("Build revision unknown")
		return
	}
	if modified {
		revision += "-dirty"
	}
	fmt.Printf("Git revision: %s\n", revision)
}
