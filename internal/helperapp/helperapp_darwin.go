//go:build darwin

package helperapp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type execRunner func(ctx context.Context, name string, args ...string) (stdout []byte, err error)

var execCommand execRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

var renamePath = os.Rename

// install copies and ad-hoc signs a helper bundle before atomically installing it.
func Install(opts InstallOptions) (err error) {
	if opts.Source == "" {
		return errors.New("helperapp: source bundle path is empty")
	}

	source, err := filepath.EvalSymlinks(opts.Source)
	if err != nil {
		return fmt.Errorf("helperapp: resolve source bundle %q: %w", opts.Source, err)
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("helperapp: inspect source bundle %q: %w", source, err)
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("helperapp: source bundle %q is not a directory", source)
	}

	dest, err := bundlePath(opts.Dest)
	if err != nil {
		return err
	}
	if samePath(source, dest) {
		return fmt.Errorf("helperapp: source and destination must differ: %q", dest)
	}

	destParent := filepath.Dir(dest)
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		return fmt.Errorf("helperapp: create bundle directory %q: %w", destParent, err)
	}

	stageRoot, err := os.MkdirTemp(destParent, "."+filepath.Base(dest)+".new.")
	if err != nil {
		return fmt.Errorf("helperapp: create staging directory in %q: %w", destParent, err)
	}
	defer func() {
		if removeErr := os.RemoveAll(stageRoot); removeErr != nil && err == nil {
			err = fmt.Errorf("helperapp: remove staging directory %q: %w", stageRoot, removeErr)
		}
	}()

	stagedBundle := filepath.Join(stageRoot, filepath.Base(dest))
	if err := copyBundle(source, stagedBundle); err != nil {
		return err
	}
	if _, err := execCommand(context.Background(), "codesign", "--force", "--sign", "-", stagedBundle); err != nil {
		return fmt.Errorf("helperapp: sign staged bundle %q: %w", stagedBundle, err)
	}

	backupPath := stageRoot + ".old"
	hasBackup := false
	if _, statErr := os.Lstat(dest); statErr == nil {
		if err := renamePath(dest, backupPath); err != nil {
			return fmt.Errorf("helperapp: move existing bundle %q aside: %w", dest, err)
		}
		hasBackup = true
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("helperapp: inspect destination bundle %q: %w", dest, statErr)
	}

	if err := renamePath(stagedBundle, dest); err != nil {
		installErr := fmt.Errorf("helperapp: install staged bundle at %q: %w", dest, err)
		if hasBackup {
			if restoreErr := renamePath(backupPath, dest); restoreErr != nil {
				return fmt.Errorf("%w; helperapp: restore previous bundle at %q: %v", installErr, dest, restoreErr)
			}
		}
		return installErr
	}

	if hasBackup {
		if err := os.RemoveAll(backupPath); err != nil {
			return fmt.Errorf("helperapp: remove previous bundle backup %q: %w", backupPath, err)
		}
	}
	return nil
}

// status reports the helper bundle's installation and signature state.
func Status(path string) (StatusResult, error) {
	bundle, err := bundlePath(path)
	if err != nil {
		return StatusResult{}, err
	}
	result := StatusResult{BundlePath: bundle}
	if _, err := os.Lstat(bundle); err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return StatusResult{}, fmt.Errorf("helperapp: inspect bundle %q: %w", bundle, err)
	}
	result.Installed = true

	var details []string
	if _, err := execCommand(context.Background(), "codesign", "--verify", "--strict", bundle); err == nil {
		result.SignatureValid = true
	} else {
		details = append(details, "signature verification failed: "+err.Error())
	}

	entitlements, err := execCommand(context.Background(), "codesign", "-d", "--entitlements", ":-", bundle)
	if err != nil {
		details = append(details, "read entitlements failed: "+err.Error())
	} else {
		result.HasEntitlements = containsEntitlementKey(entitlements)
	}
	result.Detail = strings.Join(details, "; ")
	return result, nil
}

func bundlePath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("helperapp: locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "povr", "Povr.app"), nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && leftAbs == rightAbs
}

func copyBundle(source, dest string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("helperapp: read source bundle %q: %w", path, walkErr)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return fmt.Errorf("helperapp: resolve source bundle path %q: %w", path, err)
		}
		target := filepath.Join(dest, rel)
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("helperapp: inspect source bundle path %q: %w", path, err)
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("helperapp: read source bundle symlink %q: %w", path, err)
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return fmt.Errorf("helperapp: create staged bundle symlink %q: %w", target, err)
			}
			return nil
		case entry.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("helperapp: create staged bundle directory %q: %w", target, err)
			}
			if err := os.Chmod(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("helperapp: set staged bundle directory mode %q: %w", target, err)
			}
			return nil
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("helperapp: unsupported source bundle file %q", path)
		}
	})
}

func copyFile(source, dest string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("helperapp: open source bundle file %q: %w", source, err)
	}
	defer input.Close()

	output, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("helperapp: create staged bundle file %q: %w", dest, err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("helperapp: copy staged bundle file %q: %w", dest, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("helperapp: close staged bundle file %q: %w", dest, closeErr)
	}
	if err := os.Chmod(dest, mode); err != nil {
		return fmt.Errorf("helperapp: set staged bundle file mode %q: %w", dest, err)
	}
	return nil
}

func containsEntitlementKey(output []byte) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(output)))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) || err != nil {
			return false
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "key" {
			return true
		}
	}
}
