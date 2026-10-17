package database

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

var editionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func validEdition(edition string) bool {
	return editionPattern.MatchString(edition)
}

// One marker per edition avoids lost updates between server processes.
const editionsDirectory = ".maxminddb-mcp-editions"

func (u *Updater) rememberEdition(edition string) error {
	dir := filepath.Join(u.config.MaxMind.DatabaseDir, editionsDirectory)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, edition), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		info, err := os.Lstat(filepath.Join(dir, edition))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("edition marker is not a regular file")
		}
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func (u *Updater) editions() ([]string, error) {
	editions := slices.Clone(u.config.MaxMind.Editions)
	entries, err := os.ReadDir(filepath.Join(u.config.MaxMind.DatabaseDir, editionsDirectory))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read installed editions: %w", err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && validEdition(entry.Name()) &&
			!slices.Contains(editions, entry.Name()) {
			editions = append(editions, entry.Name())
		}
	}
	return editions, nil
}
