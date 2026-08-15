package projectbrain

import (
	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/project"
)

// Collect reuses the native project map scanner and its gitignore/sensitive-path
// rules. It intentionally does not implement a second filesystem discovery path.
func Collect(root string) (Manifest, error) {
	fs, err := localfs.New(root)
	if err != nil {
		return Manifest{}, err
	}
	engine := project.New(fs)
	defer engine.Close()
	projectMap, err := engine.Map(true)
	if err != nil {
		return Manifest{}, err
	}
	return FromProjectMap(projectMap), nil
}
