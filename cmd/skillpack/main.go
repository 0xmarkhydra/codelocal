package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "skillpack:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("skillpack", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "path to the skill manifest JSON")
	root := flags.String("root", "", "root directory of the pinned source snapshot")
	output := flags.String("out", "", "output package JSON path; stdout when omitted")
	pretty := flags.Bool("pretty", true, "pretty-print package JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*manifestPath) == "" || strings.TrimSpace(*root) == "" {
		return errors.New("-manifest and -root are required")
	}

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	documents, err := readSourceDocuments(*root)
	if err != nil {
		return err
	}
	artifact, err := skills.BuildArtifactFromDocuments(manifest, documents, skills.DefaultKnowledgeIngestPolicy())
	if err != nil {
		return err
	}
	pkg, err := skills.BuildPackage(manifest, artifact)
	if err != nil {
		return err
	}
	payload, err := marshalPackage(pkg, *pretty)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*output) == "" {
		_, err = stdout.Write(append(payload, '\n'))
		return err
	}
	return writeAtomic(*output, append(payload, '\n'))
}

func readManifest(manifestPath string) (skills.Manifest, error) {
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return skills.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest skills.Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return skills.Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return skills.Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}
	return manifest, nil
}

func readSourceDocuments(root string) ([]skills.SourceDocument, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat source root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source root is not a directory")
	}

	documents := []skills.SourceDocument{}
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		documents = append(documents, skills.SourceDocument{
			Path:    filepath.ToSlash(relative),
			Content: string(payload),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan source snapshot: %w", err)
	}
	return documents, nil
}

func marshalPackage(pkg skills.Package, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(pkg, "", "  ")
	}
	return json.Marshal(pkg)
}

func writeAtomic(output string, payload []byte) error {
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".skillpack-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, output); err != nil {
		return err
	}
	return nil
}
