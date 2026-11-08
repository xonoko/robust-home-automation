//go:build linux

// Command spike validates live TCP connection migration between two Linux
// nodes with TCP_REPAIR, a VIP and a virtual MAC (docs/adr/0005).
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spike node|device|migrate [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "node":
		err = runNode(os.Args[2:])
	case "device":
		err = runDevice(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runMigrate asks the local node to move its connections to the node at args[0].
func runMigrate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: spike migrate http://<target>:9000")
	}
	resp, err := http.Post("http://127.0.0.1:9000/migrate?to="+args[0], "", nil) //nolint:noctx // spike
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Print(string(body))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("migration failed: %s", resp.Status)
	}
	return nil
}
