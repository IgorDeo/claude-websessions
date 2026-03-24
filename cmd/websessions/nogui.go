//go:build !gui

package main

import "fmt"

func openGUI(_ string) error {
	return fmt.Errorf("GUI not available: rebuild with -tags gui (requires libwebkit2gtk-4.1-dev on Linux)")
}

func ensureJSCSignalEnv() {}
