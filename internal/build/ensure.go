package build

import (
	"github.com/rifanid/agentmap/internal/cache"
	"github.com/rifanid/agentmap/internal/model"
	"github.com/rifanid/agentmap/internal/resolve"
)

// Root returns the workspace root for startDir (go.work dir, nearest go.mod
// dir, or startDir itself).
func Root(startDir string) string {
	return resolve.Discover(startDir).Root
}

// EnsureFresh returns the cached map when it is provably current, else rebuilds
// (and persists). Trust rule mirrors agentmap: same HEAD + known schema + built
// clean + clean now; non-git falls back to a source fingerprint match.
func EnsureFresh(startDir string) (*model.Map, error) {
	root := Root(startDir)
	if cached, err := cache.Load(root); err == nil && cached.Schema == model.SchemaVersion {
		sha := cache.CurrentSha(root)
		if sha != "" && cached.GeneratedSha == sha && cached.Dirty == 0 && cache.DirtyCount(root) == 0 {
			return cached, nil
		}
		if sha == "" && cached.Fingerprint != "" && cached.Fingerprint == cache.Fingerprint(root) {
			return cached, nil
		}
	}
	return BuildAndSave(startDir)
}

// BuildAndSave builds the map and persists it to the cache, returning the map.
func BuildAndSave(startDir string) (*model.Map, error) {
	m, err := Build(startDir)
	if err != nil {
		return nil, err
	}
	if err := cache.Save(Root(startDir), m); err != nil {
		return nil, err
	}
	return m, nil
}
