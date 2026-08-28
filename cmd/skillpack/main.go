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
	policy := skillpackIngestPolicy()
	documents, err := readSourceDocuments(*root, policy, skills.DefaultSkillpackTotalBytes)
	if err != nil {
		return err
	}
	artifact, err := skills.BuildArtifactFromDocumentsBudgeted(manifest, documents, policy, skills.DefaultSkillpackTotalBytes)
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

func skillpackIngestPolicy() skills.IngestPolicy {
	policy := skills.DefaultKnowledgeIngestPolicy()
	policy.MaxDocumentBytes = skills.DefaultSkillpackDocumentBytes
	return policy
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

func readSourceDocuments(root string, policy skills.IngestPolicy, maxTotalBytes int) ([]skills.SourceDocument, error) {
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
	if maxTotalBytes <= 0 {
		maxTotalBytes = skills.DefaultSkillpackTotalBytes
	}

	allowedExtensions := lowerSet(policy.AllowedExtensions)
	excludedSegments := lowerSet(policy.ExcludedPathSegments)
	documents := []skills.SourceDocument{}
	var totalBytes int64
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if _, blocked := excludedSegments[strings.ToLower(entry.Name())]; blocked {
				return filepath.SkipDir
			}
			return nil
		}
		if _, allowed := allowedExtensions[strings.ToLower(filepath.Ext(entry.Name()))]; !allowed {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if fileInfo.Size() > int64(policy.MaxDocumentBytes) {
			return fmt.Errorf("skill source document %q exceeds %d bytes", filePath, policy.MaxDocumentBytes)
		}
		totalBytes += fileInfo.Size()
		if totalBytes > int64(maxTotalBytes) {
			return fmt.Errorf("skill source snapshot exceeds %d bytes of allowed text", maxTotalBytes)
		}
		if len(documents) >= policy.MaxDocuments {
			return fmt.Errorf("skill source snapshot exceeds %d documents", policy.MaxDocuments)
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

func lowerSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
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
