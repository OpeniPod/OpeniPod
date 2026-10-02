package library

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

// Filesystem loads supported audio files below a directory.
type Filesystem struct {
	root string
}

const libraryIndexVersion = 1

type libraryIndex struct {
	Version int           `json:"version"`
	Root    string        `json:"root"`
	Tracks  []music.Track `json:"tracks"`
}

// NewFilesystem creates a filesystem library rooted at root. Load resolves
// root to a cleaned absolute path. os.DirFS follows a root symlink if one was
// supplied; symlinks encountered below it are skipped.
func NewFilesystem(root string) *Filesystem {
	return &Filesystem{root: root}
}

// Load returns the saved library index, scanning the root when no index exists.
func (f *Filesystem) Load(ctx context.Context) ([]music.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	absoluteRoot, err := resolveRoot(f.root)
	if err != nil {
		return nil, fmt.Errorf("resolve library root %q: %w", f.root, err)
	}

	indexPath := filepath.Join(absoluteRoot, ".openipod-library.json")
	data, err := os.ReadFile(indexPath)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err == nil {
		tracks, decodeErr := decodeIndex(data, absoluteRoot)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return tracks, decodeErr
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read library index %s: %w", indexPath, err)
	}

	return f.Refresh(ctx)
}

// Refresh rescans the root and atomically saves the resulting library index.
func (f *Filesystem) Refresh(ctx context.Context) ([]music.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	absoluteRoot, err := resolveRoot(f.root)
	if err != nil {
		return nil, fmt.Errorf("resolve library root %q: %w", f.root, err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("stat library root %s: %w", absoluteRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("library root %s is not a directory", absoluteRoot)
	}

	return f.refresh(ctx, absoluteRoot, filepath.Join(absoluteRoot, ".openipod-library.json"))
}

func (f *Filesystem) refresh(ctx context.Context, absoluteRoot, indexPath string) ([]music.Track, error) {
	tracks, err := load(ctx, absoluteRoot, os.DirFS(absoluteRoot))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := json.Marshal(libraryIndex{
		Version: libraryIndexVersion,
		Root:    absoluteRoot,
		Tracks:  tracks,
	})
	if err != nil {
		return nil, fmt.Errorf("encode library index: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(absoluteRoot, ".openipod-library.json.tmp-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary library index: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("set temporary library index permissions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("write temporary library index: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("sync temporary library index: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close temporary library index: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, indexPath); err != nil {
		return nil, fmt.Errorf("replace library index: %w", err)
	}
	return tracks, nil
}

func resolveRoot(root string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absoluteRoot), nil
}

func decodeIndex(data []byte, absoluteRoot string) ([]music.Track, error) {
	var index libraryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("decode library index: %w", err)
	}
	if index.Version != libraryIndexVersion {
		return nil, fmt.Errorf("decode library index: unsupported version %d", index.Version)
	}
	if index.Root != absoluteRoot {
		return nil, fmt.Errorf("decode library index: root %q does not match %q", index.Root, absoluteRoot)
	}
	if index.Tracks == nil {
		return nil, fmt.Errorf("decode library index: tracks must be an array")
	}
	for i, track := range index.Tracks {
		if track.ID == "" || track.Path == "" || track.Title == "" {
			return nil, fmt.Errorf("decode library index: track %d is missing ID, path, or title", i)
		}
		relativePath, err := filepath.Rel(absoluteRoot, track.Path)
		if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.Clean(track.Path) != track.Path {
			return nil, fmt.Errorf("decode library index: track %d has an invalid path %q", i, track.Path)
		}
		if i > 0 && index.Tracks[i-1].Path >= track.Path {
			return nil, fmt.Errorf("decode library index: track paths must be unique and sorted")
		}
	}
	return index.Tracks, nil
}

// load is kept separate from the production adapter so walk and read errors
// and context transitions can be tested deterministically with another fs.FS.
func load(ctx context.Context, absoluteRoot string, files fs.FS) ([]music.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tracks := make([]music.Track, 0)
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", filepath.Join(absoluteRoot, name), walkErr)
		}
		if name == "." {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if !supportedExtension(extension) {
			return nil
		}

		absolutePath := filepath.Join(absoluteRoot, name)
		tracks = append(tracks, music.Track{
			ID:    music.TrackID(fmt.Sprintf("%x", sha256.Sum256([]byte(absolutePath)))),
			Path:  absolutePath,
			Title: entry.Name(),
		})
		return nil
	})
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].Path < tracks[j].Path
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tracks, nil
}

func supportedExtension(extension string) bool {
	switch extension {
	case ".mp3", ".flac", ".wav", ".ogg", ".m4a", ".aac":
		return true
	default:
		return false
	}
}
