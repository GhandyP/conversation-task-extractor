// Command task-extract turns a conversation into source-backed task
// candidates, and records them only when a human approves them.
package main

import (
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
