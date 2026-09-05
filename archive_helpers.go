package gobash

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/spf13/afero"
)

type virtualArchiveEntry struct {
	source string
	name   string
	info   fs.FileInfo
}

func collectArchiveEntries(e *Env, operands []string, recursive bool) ([]virtualArchiveEntry, error) {
	entries := make([]virtualArchiveEntry, 0)
	for _, operand := range operands {
		resolved := e.Resolve(operand)
		cleanOperand := path.Clean(operand)
		if cleanOperand == "." {
			info, err := e.FS.Stat(resolved)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", operand, err)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("%s: expected a directory", operand)
			}
			children, err := afero.ReadDir(e.FS, resolved)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", operand, err)
			}
			for _, child := range children {
				if err := collectArchivePath(e, path.Join(resolved, child.Name()), child.Name(), true, &entries); err != nil {
					return nil, fmt.Errorf("%s: %w", operand, err)
				}
			}
			continue
		}
		name := strings.TrimPrefix(cleanOperand, "/")
		if name == "" {
			name = path.Base(e.Dir)
		}
		if err := collectArchivePath(e, resolved, name, recursive, &entries); err != nil {
			return nil, fmt.Errorf("%s: %w", operand, err)
		}
	}
	return entries, nil
}

func collectArchivePath(e *Env, source, name string, recursive bool, entries *[]virtualArchiveEntry) error {
	info, err := e.FS.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() && !recursive {
		return fmt.Errorf("is a directory; use -r")
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported special file")
	}
	*entries = append(*entries, virtualArchiveEntry{source: source, name: name, info: info})
	if !info.IsDir() {
		return nil
	}
	children, err := afero.ReadDir(e.FS, source)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := collectArchivePath(e, path.Join(source, child.Name()), path.Join(name, child.Name()), true, entries); err != nil {
			return err
		}
	}
	return nil
}

func safeArchiveTarget(root, archiveName string) (string, error) {
	if strings.ContainsRune(archiveName, '\x00') || path.IsAbs(archiveName) {
		return "", fmt.Errorf("unsafe archive path %q", archiveName)
	}
	clean := path.Clean(strings.ReplaceAll(archiveName, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe archive path %q", archiveName)
	}
	return path.Join(root, clean), nil
}

func matchesArchiveSelection(name string, selections []string) bool {
	if len(selections) == 0 {
		return true
	}
	clean := strings.TrimSuffix(name, "/")
	for _, selection := range selections {
		selection = strings.TrimSuffix(selection, "/")
		if clean == selection || strings.HasPrefix(clean, selection+"/") {
			return true
		}
		if matched, _ := path.Match(selection, clean); matched {
			return true
		}
	}
	return false
}
