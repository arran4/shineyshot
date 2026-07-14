#!/bin/bash
sed -i 's/fmt.Fprintf(os.Stdout, "Installing skill/_, _ = fmt.Fprintf(os.Stdout, "Installing skill/g' cmd/shineyshot/skill.go
sed -i 's/fmt.Fprintf(os.Stdout, "Skill.*installed successfully/_, _ = fmt.Fprintf(os.Stdout, "Skill '%s' installed successfully/g' cmd/shineyshot/skill.go
sed -i 's/fmt.Fprintf(os.Stdout, "Updating skill/_, _ = fmt.Fprintf(os.Stdout, "Updating skill/g' cmd/shineyshot/skill.go
sed -i 's/fmt.Fprintln(os.Stdout, string(b))/_, _ = fmt.Fprintln(os.Stdout, string(b))/g' cmd/shineyshot/skill.go
sed -i 's/fmt.Fprintln(os.Stdout, "No skills installed.")/_, _ = fmt.Fprintln(os.Stdout, "No skills installed.")/g' cmd/shineyshot/skill.go
