package main

import "fmt"

type versionCmd struct{ r *root }

func (c *versionCmd) Run() error {
	fmt.Printf("%s version %s\n", c.r.program, version)
	return nil
}
