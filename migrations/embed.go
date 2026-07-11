// Package migrations embeds every module's SQL migrations so the runner (and
// testkit) can apply them without filesystem access at runtime.
//
// Layout: one subdirectory per module (migrations/<module>/NNNNN_*.sql), each
// versioned independently by goose with its own version table. The platform
// module always migrates first — it creates the per-module schemas everything
// else depends on.
package migrations

import (
	"embed"
	"io/fs"
	"sort"
)

// FS holds every module's migration files (migrations/<module>/NNNNN_*.sql).
//
//go:embed */*.sql
var FS embed.FS

// Modules returns the module directories present in the embedded FS, with
// "platform" first (it owns the schema-creation migration) and the rest in
// alphabetical order.
func Modules() ([]string, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, err
	}

	var mods []string
	for _, e := range entries {
		if e.IsDir() {
			mods = append(mods, e.Name())
		}
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i] == "platform" {
			return true
		}
		if mods[j] == "platform" {
			return false
		}
		return mods[i] < mods[j]
	})
	return mods, nil
}
