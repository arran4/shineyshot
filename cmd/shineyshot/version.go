package main

import "fmt"

type versionCmd struct{ r *root }

func (v *versionCmd) Run() error {
	info := fmt.Sprintf("%s version %s", v.r.program, version)
	switch {
	case commit != "" && date != "":
		info = fmt.Sprintf("%s (commit %s, built %s)", info, commit, date)
	case commit != "":
		info = fmt.Sprintf("%s (commit %s)", info, commit)
	case date != "":
		info = fmt.Sprintf("%s (built %s)", info, date)
	}
	fmt.Println(info)
	return nil
}
