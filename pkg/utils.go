package pkg

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(in *os.File) {
		err := in.Close()
		if err != nil {
			log.Print(err)
		}
	}(in)

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			log.Print(err)
		}
	}(out)

	_, err = io.Copy(out, in)
	return err
}

func copyGoFiles(srcRoot, dstRoot string) error {
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstRoot, rel)

		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		if filepath.Ext(info.Name()) != ".go" {
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		return copyFile(path, dst)
	})
}

func tidy(commandDir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = commandDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed in %s: %w", commandDir, err)
	}
	return nil
}

func getUpstreamModulePath(sourceDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(sourceDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read upstream go.mod: %w", err)
	}

	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("parse upstream go.mod: %w", err)
	}
	if mf.Module == nil || mf.Module.Mod.Path == "" {
		return "", fmt.Errorf("upstream go.mod has no module path")
	}
	return mf.Module.Mod.Path, nil
}

// destDir is the root directory of the copied api modules
func collectInternalImports(destDir, upstreamModule string) ([]string, error) {
	internal := map[string]struct{}{}

	fset := token.NewFileSet()

	err := filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		// parse imports only
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		for _, imp := range f.Imports {
			raw := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(raw, upstreamModule+"/") {
				internal[raw] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var result []string
	for k := range internal {
		result = append(result, k)
	}
	return result, nil
}

func copyInternalPackages(internal []string, upstreamModule, sourceDir, destDir string) error {
	for _, imp := range internal {
		rel := strings.TrimPrefix(imp, upstreamModule+"/") // e.g. "internal/components"
		src := filepath.Join(sourceDir, rel)
		dst := filepath.Join(destDir, rel)

		info, err := os.Stat(src)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("expected internal package dir at %s for import %s", src, imp)
		}

		log.Printf("Copying internal package %s -> %s", src, dst)
		if err := copyGoFiles(src, dst); err != nil {
			return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
		}
	}
	return nil
}

func rewriteImports(apisDir, upstreamModule, mirrorModule string) error {
	oldPrefix := upstreamModule + "/"
	newPrefix := mirrorModule + "/"

	return filepath.Walk(apisDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(data)
		if !strings.Contains(s, oldPrefix) {
			return nil
		}
		s = strings.ReplaceAll(s, oldPrefix, newPrefix)
		return os.WriteFile(path, []byte(s), info.Mode())
	})
}

func tagExists(tag string) (bool, error) {
	ref := fmt.Sprintf("refs/tags/%s", tag)
	cmd := exec.Command("git", "show-ref", "--tags", "--verify", "--quiet", ref)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Exit code 0: tag exists
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 1:
			// Exit code 1 from `git show-ref --verify` => ref does NOT exist.
			// This is the normal "tag not found" case, not an error.
			return false, nil
		default:
			// Other non-zero exit code → real problem.
			return false, fmt.Errorf(
				"git show-ref failed for %s (exit=%d, stderr=%q): %w",
				ref, exitErr.ExitCode(), stderr.String(), err,
			)
		}
	}

	// Non-ExitError (e.g. context canceled, binary missing, etc.)
	return false, fmt.Errorf(
		"failed to run git show-ref for %s (stderr=%q): %w",
		ref, stderr.String(), err,
	)
}

func createTag(tag string) error {
	cmd := exec.Command("git", "tag", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create git tag %s: %w", tag, err)
	}

	return nil
}
