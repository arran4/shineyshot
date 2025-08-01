package main

import "fmt"

type versionCmd struct{ r *root }

func (v *versionCmd) Run() error {
	fmt.Printf("%s version %s\n", v.r.program, version)
	return nil
}
