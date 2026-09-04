package gobash

import (
	"context"
	"crypto/rand"
	"fmt"
	"path"
	"strings"
)

func init() { Register("mktemp", cmdMktemp) }

const tempAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func cmdMktemp(_ context.Context, e *Env) int {
	directory := false
	template := "/tmp/tmp.XXXXXXXXXX"
	for _, arg := range e.Args[1:] {
		switch arg {
		case "-d", "--directory":
			directory = true
		case "--help":
			_, _ = fmt.Fprintln(e.Stdout, "usage: mktemp [-d] [template-with-XXX]")
			return 0
		default:
			template = arg
		}
	}
	if !path.IsAbs(template) {
		template = e.Resolve(template)
	}
	lastSlash := strings.LastIndex(template, "/")
	name := template[lastSlash+1:]
	firstX := strings.Index(name, "XXX")
	if firstX < 0 {
		e.Errorf("template must contain at least three consecutive Xs")
		return 1
	}
	start := lastSlash + 1 + firstX
	end := start
	for end < len(template) && template[end] == 'X' {
		end++
	}
	for attempt := 0; attempt < 100; attempt++ {
		suffix, err := randomTempSuffix(end - start)
		if err != nil {
			e.Errorf("%v", err)
			return 1
		}
		candidate := template[:start] + suffix + template[end:]
		if _, err := e.FS.Stat(candidate); err == nil {
			continue
		}
		if directory {
			err = e.FS.Mkdir(candidate, 0o700)
		} else {
			file, createErr := e.FS.Create(candidate)
			err = createErr
			if err == nil {
				err = file.Close()
			}
		}
		if err != nil {
			e.Errorf("cannot create %s: %v", candidate, err)
			return 1
		}
		if _, err := fmt.Fprintln(e.Stdout, candidate); err != nil {
			e.Errorf("%v", err)
			return 1
		}
		return 0
	}
	e.Errorf("cannot create unique temporary path")
	return 1
}

func randomTempSuffix(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = tempAlphabet[int(bytes[i])%len(tempAlphabet)]
	}
	return string(bytes), nil
}
