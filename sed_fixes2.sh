#!/bin/bash
sed -i 's/os.RemoveAll(tmpDest)/_ = os.RemoveAll(tmpDest)/g' internal/skill/installer.go
sed -i 's/defer srcFile.Close()/defer func() { _ = srcFile.Close() }()/g' internal/skill/installer.go
sed -i 's/defer destFile.Close()/defer func() { _ = destFile.Close() }()/g' internal/skill/installer.go
sed -i 's/os.MkdirAll(skillSrcDir, 0755)/_ = os.MkdirAll(skillSrcDir, 0755)/g' internal/skill/skill_test.go
sed -i 's/os.WriteFile(filepath.Join(skillSrcDir, "SKILL.md"), \[\]byte("# Dummy Skill"), 0644)/_ = os.WriteFile(filepath.Join(skillSrcDir, "SKILL.md"), []byte("# Dummy Skill"), 0644)/g' internal/skill/skill_test.go
sed -i 's/os.WriteFile(filepath.Join(skillSrcDir, "extra.txt"), \[\]byte("extra"), 0644)/_ = os.WriteFile(filepath.Join(skillSrcDir, "extra.txt"), []byte("extra"), 0644)/g' internal/skill/skill_test.go
sed -i 's/os.MkdirAll(targetBase, 0755)/_ = os.MkdirAll(targetBase, 0755)/g' internal/skill/skill_test.go
