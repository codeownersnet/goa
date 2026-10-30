package pathguard

import (
	"fmt"
	"path/filepath"
	"strings"
)

type PathGuard struct {
	allowed []string
}

func New(allowed []string) *PathGuard {
	return &PathGuard{allowed: allowed}
}

func (g *PathGuard) Allowed(path string) bool {
	if len(g.allowed) == 0 {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	evalAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		evalAbs = abs
	}
	for _, pattern := range g.allowed {
		dir := strings.TrimSuffix(pattern, "/**")
		dir = strings.TrimSuffix(dir, "/*")
		if dir != pattern {
			evalDir, err := filepath.EvalSymlinks(dir)
			if err != nil {
				evalDir = dir
			}
			if strings.HasPrefix(evalAbs, evalDir+string(filepath.Separator)) || evalAbs == evalDir {
				return true
			}
			if strings.HasPrefix(abs, dir+string(filepath.Separator)) || abs == dir {
				return true
			}
		}
	}
	for _, pattern := range g.allowed {
		matched, err := filepath.Match(pattern, abs)
		if err == nil && matched {
			return true
		}
		if !strings.ContainsRune(pattern, filepath.Separator) {
			matched, err = filepath.Match(pattern, filepath.Base(abs))
			if err == nil && matched {
				return true
			}
		}
	}
	for _, pattern := range g.allowed {
		matched, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		subWild := strings.HasSuffix(pattern, string(filepath.Separator)+"**") ||
			strings.HasSuffix(pattern, string(filepath.Separator)+"*")
		for _, m := range matched {
			if abs == m {
				return true
			}
			if subWild && strings.HasPrefix(abs, m+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func (g *PathGuard) Check(path string) error {
	if !g.Allowed(path) {
		return fmt.Errorf("path %q is not in the allowed list", path)
	}
	return nil
}
