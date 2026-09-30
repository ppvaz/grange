// Package config loads grange settings from the environment and .env files.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads workDir/.env, or grangeHome/.env when the project has none.
// Variables already in the environment win, so `GROW_MODE=pair ./grow.sh`
// overrides the file.
func LoadDotEnv(workDir, grangeHome string) error {
	for _, dir := range []string{workDir, grangeHome} {
		f, err := os.Open(filepath.Join(dir, ".env"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		defer f.Close()
		return applyDotEnv(f)
	}
	return nil
}

func applyDotEnv(f *os.File) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

// parseLine understands KEY=value, export KEY=value, quoted values, trailing
// comments on unquoted values, and $VAR expansion outside single quotes.
func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, ok = strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	switch {
	case len(value) >= 2 && value[0] == '\'' && strings.HasSuffix(value, "'"):
		return key, value[1 : len(value)-1], true
	case len(value) >= 2 && value[0] == '"' && strings.HasSuffix(value, `"`):
		value = value[1 : len(value)-1]
	default:
		if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
	}
	return key, os.ExpandEnv(value), true
}
