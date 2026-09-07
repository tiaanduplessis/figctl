// Package cache is the disk cache for Figma responses and downloaded
// assets. Entries live under $XDG_CACHE_HOME/figctl/<profile>/<fileKey>/
// and are validated against the file's last_touched_at, which is checked
// at most once per TTL through the cheap file meta endpoint.
package cache

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

const (
	appDir     = "figctl"
	entriesDir = "entries"
	blobsDir   = "blobs"
	stateFile  = "state.json"
	entryExt   = ".json.gz"
	metaExt    = ".meta.json"
	// DefaultTTL is how long a meta check stays valid.
	DefaultTTL = 60 * time.Second
)

// Dir returns the cache root: $XDG_CACHE_HOME/figctl or ~/.cache/figctl.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, appDir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", figctl.Wrap(figctl.CodeInternal, err, "locating home directory: "+err.Error()).
			WithHint("Set XDG_CACHE_HOME or HOME.")
	}
	return filepath.Join(home, ".cache", appDir), nil
}

// Options configure a Cache.
type Options struct {
	// Root overrides the cache root (default Dir()).
	Root string
	// Profile scopes entries to an account. Required.
	Profile string
	// NoCache bypasses reads and writes.
	NoCache bool
	// Refresh bypasses reads but still writes.
	Refresh bool
	// TTL is how long a meta check stays valid. Zero means DefaultTTL.
	TTL time.Duration
	// Now overrides the clock, for tests.
	Now func() time.Time
}

// Cache is the per-profile disk cache.
type Cache struct {
	root    string
	profile string
	noCache bool
	refresh bool
	ttl     time.Duration
	now     func() time.Time
}

// Open prepares a cache for a profile. Nothing is written until the first
// Put.
func Open(opts Options) (*Cache, error) {
	if err := checkSegment(opts.Profile, "profile"); err != nil {
		return nil, err
	}
	root := opts.Root
	if root == "" {
		var err error
		root, err = Dir()
		if err != nil {
			return nil, err
		}
	}
	c := &Cache{
		root:    root,
		profile: opts.Profile,
		noCache: opts.NoCache,
		refresh: opts.Refresh,
		ttl:     opts.TTL,
		now:     opts.Now,
	}
	if c.ttl <= 0 {
		c.ttl = DefaultTTL
	}
	if c.now == nil {
		c.now = time.Now
	}
	return c, nil
}

// Root returns the cache root directory.
func (c *Cache) Root() string { return c.root }

// Disabled reports whether --no-cache is in effect.
func (c *Cache) Disabled() bool { return c.noCache }

// Refreshing reports whether --refresh is in effect.
func (c *Cache) Refreshing() bool { return c.refresh }

func checkSegment(s, what string) error {
	if s == "" || strings.ContainsAny(s, `/\`) || s == "." || s == ".." {
		return figctl.Newf(figctl.CodeInternal, "invalid cache %s %q", what, s)
	}
	return nil
}

// Stamp identifies the file state an entry was fetched at. Entries are
// reused only while the stamp matches.
type Stamp struct {
	Version       string `json:"version,omitempty"`
	LastTouchedAt string `json:"lastTouchedAt,omitempty"`
}

// State is the per-file validation state.
type State struct {
	Stamp     Stamp     `json:"stamp"`
	CheckedAt time.Time `json:"checkedAt"`
	// TooLarge records that the whole document exceeded --max-file-mb, so
	// later runs skip the full fetch.
	TooLarge bool `json:"tooLarge,omitempty"`
}

// Key builds an entry key from an endpoint name and its query parameters,
// normalized so that parameter order does not matter.
func Key(endpoint string, query url.Values) string {
	if len(query) == 0 {
		return endpoint
	}
	return endpoint + "?" + query.Encode()
}

// BlobKey builds the key of a rendered image or downloaded asset.
func BlobKey(nodeID, format string, scale float64, version string) string {
	return "render/" + nodeID + "/" + format + "/" + strconv.FormatFloat(scale, 'f', -1, 64) + "/" + version
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

func (c *Cache) fileDir(fileKey string) (string, error) {
	if err := checkSegment(fileKey, "file key"); err != nil {
		return "", err
	}
	return filepath.Join(c.root, c.profile, fileKey), nil
}

func (c *Cache) ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "creating cache directory: "+err.Error())
	}
	return nil
}

// FileState reads the validation state of a file. A missing state is
// returned as the zero value.
func (c *Cache) FileState(fileKey string) (State, error) {
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return State{}, err
	}
	var state State
	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, figctl.Wrap(figctl.CodeInternal, err, "reading cache state: "+err.Error())
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, nil
	}
	return state, nil
}

func (c *Cache) writeState(fileKey string, state State) error {
	if c.noCache {
		return nil
	}
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return err
	}
	if err := c.ensureDir(dir); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding cache state: "+err.Error())
	}
	return writeFile(filepath.Join(dir, stateFile), raw)
}

// SetTooLarge records whether the whole document is too large to cache.
func (c *Cache) SetTooLarge(fileKey string, tooLarge bool) error {
	state, err := c.FileState(fileKey)
	if err != nil {
		return err
	}
	state.TooLarge = tooLarge
	return c.writeState(fileKey, state)
}

// Validate returns the current stamp of a file. It calls fetch (the file
// meta endpoint) at most once per TTL and otherwise reuses the stored
// stamp. With --refresh or --no-cache it always calls fetch.
func (c *Cache) Validate(ctx context.Context, fileKey string, fetch func(ctx context.Context) (Stamp, error)) (Stamp, error) {
	state, err := c.FileState(fileKey)
	if err != nil {
		return Stamp{}, err
	}
	now := c.now()
	fresh := !c.noCache && !c.refresh && !state.CheckedAt.IsZero() && now.Sub(state.CheckedAt) < c.ttl && state.Stamp != (Stamp{})
	if fresh {
		return state.Stamp, nil
	}
	stamp, err := fetch(ctx)
	if err != nil {
		return Stamp{}, err
	}
	state.Stamp = stamp
	state.CheckedAt = now
	if err := c.writeState(fileKey, state); err != nil {
		return Stamp{}, err
	}
	return stamp, nil
}

type header struct {
	Key       string    `json:"key"`
	Stamp     Stamp     `json:"stamp"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// Get returns the cached data for key when it was stored at the same
// stamp. It reports a miss with --refresh or --no-cache.
func (c *Cache) Get(fileKey, key string, stamp Stamp) ([]byte, bool, error) {
	if c.noCache || c.refresh {
		return nil, false, nil
	}
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, entriesDir, hashKey(key)+entryExt)
	h, data, err := readEntry(path, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, false, nil
	}
	if h.Stamp != stamp || h.Key != key {
		_ = os.Remove(path)
		return nil, false, nil
	}
	return data, true, nil
}

// Put stores data for key at the given stamp. It is a no-op with
// --no-cache.
func (c *Cache) Put(fileKey, key string, stamp Stamp, data []byte) error {
	if c.noCache {
		return nil
	}
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, entriesDir)
	if err := c.ensureDir(dir); err != nil {
		return err
	}
	h, err := json.Marshal(header{Key: key, Stamp: stamp, FetchedAt: c.now()})
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding cache entry: "+err.Error())
	}
	path := filepath.Join(dir, hashKey(key)+entryExt)
	tmp, err := os.CreateTemp(dir, "entry-*.tmp")
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "writing cache entry: "+err.Error())
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	zw := gzip.NewWriter(tmp)
	_, werr := zw.Write(append(h, '\n'))
	if werr == nil {
		_, werr = zw.Write(data)
	}
	if cerr := zw.Close(); werr == nil {
		werr = cerr
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return figctl.Wrap(figctl.CodeInternal, werr, "writing cache entry: "+werr.Error())
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "writing cache entry: "+err.Error())
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "writing cache entry: "+err.Error())
	}
	return nil
}

// readEntry reads an entry header and, when withData is set, its data.
func readEntry(path string, withData bool) (header, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return header{}, nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return header{}, nil, err
	}
	defer func() { _ = zr.Close() }()
	br := bufio.NewReader(zr)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return header{}, nil, err
	}
	var h header
	if err := json.Unmarshal(line, &h); err != nil {
		return header{}, nil, err
	}
	if !withData {
		return h, nil, nil
	}
	data, err := io.ReadAll(br)
	if err != nil {
		return header{}, nil, err
	}
	return h, data, nil
}

// GetBlob returns a cached asset. Blobs are keyed by node, format, scale,
// and version (see BlobKey), so they never go stale and need no stamp.
func (c *Cache) GetBlob(fileKey, key string) ([]byte, bool, error) {
	if c.noCache || c.refresh {
		return nil, false, nil
	}
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(filepath.Join(dir, blobsDir, hashKey(key)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, figctl.Wrap(figctl.CodeInternal, err, "reading cached asset: "+err.Error())
	}
	return data, true, nil
}

type blobMeta struct {
	Key      string    `json:"key"`
	StoredAt time.Time `json:"storedAt"`
}

// PutBlob stores an asset. It is a no-op with --no-cache.
func (c *Cache) PutBlob(fileKey, key string, data []byte) error {
	if c.noCache {
		return nil
	}
	dir, err := c.fileDir(fileKey)
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, blobsDir)
	if err := c.ensureDir(dir); err != nil {
		return err
	}
	meta, err := json.Marshal(blobMeta{Key: key, StoredAt: c.now()})
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "encoding asset metadata: "+err.Error())
	}
	name := hashKey(key)
	if err := writeFile(filepath.Join(dir, name), data); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, name+metaExt), meta)
}

func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "writing cache file: "+err.Error())
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		return figctl.Wrap(figctl.CodeInternal, werr, "writing cache file: "+werr.Error())
	}
	return nil
}

// EntryInfo describes one cached entry for status output.
type EntryInfo struct {
	Key       string    `json:"key"`
	Bytes     int64     `json:"bytes"`
	FetchedAt time.Time `json:"fetchedAt"`
	Version   string    `json:"version,omitempty"`
}

// FileStatus summarizes the cache of one file.
type FileStatus struct {
	Key           string      `json:"key"`
	Bytes         int64       `json:"bytes"`
	Entries       []EntryInfo `json:"entries"`
	Blobs         int         `json:"blobs"`
	Version       string      `json:"version,omitempty"`
	LastTouchedAt string      `json:"lastTouchedAt,omitempty"`
	CheckedAt     *time.Time  `json:"checkedAt,omitempty"`
	TooLarge      bool        `json:"tooLarge,omitempty"`
}

// ProfileStatus summarizes the cache of one profile.
type ProfileStatus struct {
	Name  string       `json:"name"`
	Bytes int64        `json:"bytes"`
	Files []FileStatus `json:"files"`
}

// Status summarizes the whole cache.
type Status struct {
	Root     string          `json:"root"`
	Bytes    int64           `json:"bytes"`
	Profiles []ProfileStatus `json:"profiles"`
}

// Scan reads the cache directory tree under root.
func Scan(root string) (*Status, error) {
	status := &Status{Root: root, Profiles: []ProfileStatus{}}
	profiles, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "reading cache directory: "+err.Error())
	}
	for _, p := range profiles {
		if !p.IsDir() {
			continue
		}
		ps := ProfileStatus{Name: p.Name(), Files: []FileStatus{}}
		files, err := os.ReadDir(filepath.Join(root, p.Name()))
		if err != nil {
			return nil, figctl.Wrap(figctl.CodeInternal, err, "reading cache directory: "+err.Error())
		}
		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			fs, err := scanFile(filepath.Join(root, p.Name(), f.Name()), f.Name())
			if err != nil {
				return nil, err
			}
			ps.Files = append(ps.Files, fs)
			ps.Bytes += fs.Bytes
		}
		sort.Slice(ps.Files, func(i, j int) bool { return ps.Files[i].Key < ps.Files[j].Key })
		status.Profiles = append(status.Profiles, ps)
		status.Bytes += ps.Bytes
	}
	sort.Slice(status.Profiles, func(i, j int) bool { return status.Profiles[i].Name < status.Profiles[j].Name })
	return status, nil
}

func scanFile(dir, key string) (FileStatus, error) {
	fs := FileStatus{Key: key, Entries: []EntryInfo{}}
	if raw, err := os.ReadFile(filepath.Join(dir, stateFile)); err == nil {
		var state State
		if json.Unmarshal(raw, &state) == nil {
			fs.Version = state.Stamp.Version
			fs.LastTouchedAt = state.Stamp.LastTouchedAt
			fs.TooLarge = state.TooLarge
			if !state.CheckedAt.IsZero() {
				checked := state.CheckedAt
				fs.CheckedAt = &checked
			}
		}
		fs.Bytes += int64(len(raw))
	}
	entries, _ := os.ReadDir(filepath.Join(dir, entriesDir))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || !strings.HasSuffix(e.Name(), entryExt) {
			continue
		}
		fs.Bytes += info.Size()
		h, _, err := readEntry(filepath.Join(dir, entriesDir, e.Name()), false)
		if err != nil {
			continue
		}
		fs.Entries = append(fs.Entries, EntryInfo{Key: h.Key, Bytes: info.Size(), FetchedAt: h.FetchedAt, Version: h.Stamp.Version})
	}
	sort.Slice(fs.Entries, func(i, j int) bool { return fs.Entries[i].Key < fs.Entries[j].Key })
	blobs, _ := os.ReadDir(filepath.Join(dir, blobsDir))
	for _, b := range blobs {
		info, err := b.Info()
		if err != nil || b.IsDir() {
			continue
		}
		fs.Bytes += info.Size()
		if !strings.HasSuffix(b.Name(), metaExt) {
			fs.Blobs++
		}
	}
	return fs, nil
}

// Clear removes cached data under root. An empty profile clears every
// profile; an empty fileKey clears every file of the selected profiles.
// It returns the number of bytes removed.
func Clear(root, profile, fileKey string) (int64, error) {
	status, err := Scan(root)
	if err != nil {
		return 0, err
	}
	var removed int64
	for _, p := range status.Profiles {
		if profile != "" && p.Name != profile {
			continue
		}
		if fileKey == "" {
			if err := os.RemoveAll(filepath.Join(root, p.Name)); err != nil {
				return removed, figctl.Wrap(figctl.CodeInternal, err, "clearing cache: "+err.Error())
			}
			removed += p.Bytes
			continue
		}
		for _, f := range p.Files {
			if f.Key != fileKey {
				continue
			}
			if err := os.RemoveAll(filepath.Join(root, p.Name, f.Key)); err != nil {
				return removed, figctl.Wrap(figctl.CodeInternal, err, "clearing cache: "+err.Error())
			}
			removed += f.Bytes
		}
	}
	return removed, nil
}

// FormatBytes renders a byte count for humans.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
