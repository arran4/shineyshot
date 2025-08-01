package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
)

type interactiveCmd struct {
	r *root
}

func (i *interactiveCmd) Run() error {
	// Create shared application state for interactive operations
	i.r.state = appstate.New()
	fmt.Fprintln(os.Stdout, "Enter commands (type 'exit' to quit)")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" {
			break
		}
		args := strings.Fields(line)
		if len(args) == 0 {
			continue
		}
		if args[0] == "interactive" {
			continue
		}
		if err := i.r.Run(args); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	return scanner.Err()
}
