package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/ref"
)

// errTooLarge marks a file whose whole document exceeds --max-file-mb.
var errTooLarge = errors.New("file document exceeds --max-file-mb")

// Session bundles the resolved profile, the API client, and the cache for
// one command. It implements the whole-file strategy: the full document is
// fetched once per file version and later queries are served locally.
type Session struct {
	ctx     *Context
	Config  *config.Config
	Active  *config.Active
	Client  *figma.Client
	Cache   *cache.Cache
	version string
	files   map[string]*fileState
}

type fileState struct {
	stamp    cache.Stamp
	stamped  bool
	full     *figma.GetFileResponse
	raw      json.RawMessage
	tooLarge bool
}

// Session resolves the active profile, builds the client and cache, and
// records the profile in the envelope.
func (c *Context) Session() (*Session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	opts, err := c.ResolveOptions(true)
	if err != nil {
		return nil, err
	}
	active, err := config.Resolve(cfg, opts)
	if err != nil {
		return nil, err
	}
	c.SetProfile(active)
	client := c.newClient(active.Token, active.Profile.BaseURL, func() []string {
		var names []string
		for _, name := range cfg.Names() {
			if name != active.Name {
				names = append(names, name)
			}
		}
		return names
	})
	root, err := cache.Dir()
	if err != nil {
		return nil, err
	}
	store, err := cache.Open(cache.Options{
		Root:    root,
		Profile: active.Name,
		NoCache: c.Flags.NoCache,
		Refresh: c.Flags.Refresh,
		TTL:     c.Flags.CacheTTL,
	})
	if err != nil {
		return nil, err
	}
	return &Session{
		ctx:     c,
		Config:  cfg,
		Active:  active,
		Client:  client,
		Cache:   store,
		version: c.Flags.FileVersion,
		files:   map[string]*fileState{},
	}, nil
}

// newClient builds a Figma client honoring --timeout, --verbose, the
// profile base URL, and FIGMA_API_BASE_URL.
func (c *Context) newClient(token, baseURL string, profileNames func() []string) *figma.Client {
	if env := os.Getenv(figma.BaseURLEnv); env != "" {
		baseURL = env
	}
	var logger figma.Logger
	if c.Flags.Verbose {
		logger = c.Log
	}
	return figma.New(figma.Options{
		Token:        token,
		BaseURL:      baseURL,
		Version:      version,
		Timeout:      c.Flags.Timeout,
		Logger:       logger,
		ProfileNames: profileNames,
	})
}

// Ref parses a file reference. A version-id in the URL pins the file
// version unless --file-version was given.
func (s *Session) Ref(arg string) (ref.Ref, error) {
	project := s.project()
	arg = strings.TrimSpace(arg)

	// With no ref at all, fall back to the file the repository nominates.
	if arg == "" {
		key, ok := project.DefaultKey()
		if !ok {
			return ref.Ref{}, s.noRefError(project)
		}
		return s.trackVersion(ref.Ref{FileKey: key, Kind: ref.KindKey}), nil
	}

	// A configured name stands in for its key, so commands read as
	// "tokens export design-system" rather than carrying a key around.
	if key, ok := project.Lookup(arg); ok {
		return s.trackVersion(ref.Ref{FileKey: key, Kind: ref.KindKey}), nil
	}

	r, err := ref.Parse(arg)
	if err != nil {
		// A word that is neither a key nor a URL was probably meant as a
		// name, so say which names exist rather than explaining ref syntax.
		if names := project.Names(); len(names) > 0 && !strings.Contains(arg, "/") {
			return ref.Ref{}, figctl.Newf(figctl.CodeUsage, "%q is not a file key, a Figma URL, or a file configured in %s", arg, project.Path).
				WithHint("Configured files: %s.", strings.Join(names, ", "))
		}
		return ref.Ref{}, err
	}
	return s.trackVersion(r), nil
}

// refArg returns the ref a command was given, or empty when it was omitted so
// the repository's default applies.
func refArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// noRefError explains what to pass when a command was given no ref and the
// repository nominates no default.
func (s *Session) noRefError(project *config.Project) error {
	e := figctl.New(figctl.CodeUsage, "no Figma file given")
	switch names := project.Names(); {
	case len(names) > 1:
		return e.WithHint("Name one of the files configured in %s (%s), or pass a file key or Figma URL. Set a default with figctl init --default <name>.",
			project.Path, strings.Join(names, ", "))
	case len(names) == 1:
		return e.WithHint("Pass %s, a file key, or a Figma URL.", names[0])
	default:
		return e.WithHint("Pass a file key or a Figma URL, or configure this repository with figctl init --file <name>=<key>.")
	}
}

// speculativeTimeout caps the whole-document fetch. A third of the budget is
// enough for the small files where it succeeds, and leaves the rest for the
// narrower requests that follow when it does not.
func (s *Session) speculativeTimeout() time.Duration {
	budget := s.ctx.Flags.Timeout
	if budget <= 0 {
		return 20 * time.Second
	}
	capped := budget / 3
	if capped < 10*time.Second {
		capped = 10 * time.Second
	}
	if capped > budget {
		capped = budget
	}
	return capped
}

// project returns the repository config backing the active profile, which is
// nil when the command runs outside a configured repository.
func (s *Session) project() *config.Project {
	if s.Active == nil {
		return nil
	}
	return s.Active.Project
}

func (s *Session) trackVersion(r ref.Ref) ref.Ref {
	if s.version == "" && r.VersionID != "" {
		s.version = r.VersionID
	}
	return r
}

// Version returns the pinned file version, or empty.
func (s *Session) Version() string { return s.version }

func (s *Session) state(key string) *fileState {
	st, ok := s.files[key]
	if !ok {
		st = &fileState{}
		s.files[key] = st
	}
	return st
}

// Stamp returns the cache stamp of a file: the pinned version when one is
// set, otherwise the last_touched_at from the meta endpoint, checked at
// most once per --cache-ttl.
func (s *Session) Stamp(ctx context.Context, key string) (cache.Stamp, error) {
	st := s.state(key)
	if st.stamped {
		return st.stamp, nil
	}
	if s.version != "" {
		st.stamp = cache.Stamp{Version: s.version}
		st.stamped = true
		return st.stamp, nil
	}
	if s.Cache.Disabled() {
		st.stamped = true
		return st.stamp, nil
	}
	stamp, err := s.Cache.Validate(ctx, key, func(ctx context.Context) (cache.Stamp, error) {
		meta, err := s.Client.GetFileMeta(ctx, key)
		if err != nil {
			return cache.Stamp{}, err
		}
		return cache.Stamp{Version: meta.File.Version, LastTouchedAt: meta.File.LastTouchedAt}, nil
	})
	if err != nil {
		return cache.Stamp{}, err
	}
	st.stamp = stamp
	st.stamped = true
	return stamp, nil
}

// cached serves entryKey from the cache or calls fetch and stores the
// encoded result.
func cached[T any](ctx context.Context, s *Session, key, entryKey string, fetch func(ctx context.Context) (*T, error)) (*T, error) {
	stamp, err := s.Stamp(ctx, key)
	if err != nil {
		return nil, err
	}
	if data, ok, err := s.Cache.Get(key, entryKey, stamp); err != nil {
		return nil, err
	} else if ok {
		var out T
		if err := json.Unmarshal(data, &out); err == nil {
			s.ctx.Log.Debugf("cache hit %s for %s", entryKey, key)
			return &out, nil
		}
	}
	out, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "encoding cache entry: "+err.Error())
	}
	if err := s.Cache.Put(key, entryKey, stamp, data); err != nil {
		s.ctx.Log.Warnf("cache write failed: %v", err)
	}
	return out, nil
}

// File returns the whole document of a file, fetched once per version and
// cached. It returns errTooLarge when the document exceeds --max-file-mb
// and the caller must fall back to narrower requests.
func (s *Session) File(ctx context.Context, key string) (*figma.GetFileResponse, error) {
	st := s.state(key)
	if st.full != nil {
		return st.full, nil
	}
	raw, err := s.FileRaw(ctx, key)
	if err != nil {
		return nil, err
	}
	var file figma.GetFileResponse
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding file document: "+err.Error())
	}
	st.full = &file
	return &file, nil
}

// FileRaw returns the whole document of a file as Figma sent it, using
// the same cache entry as File. It returns errTooLarge when the document
// exceeds --max-file-mb.
func (s *Session) FileRaw(ctx context.Context, key string) (json.RawMessage, error) {
	st := s.state(key)
	if st.raw != nil {
		return st.raw, nil
	}
	if st.tooLarge {
		return nil, errTooLarge
	}
	stamp, err := s.Stamp(ctx, key)
	if err != nil {
		return nil, err
	}
	if !s.Cache.Refreshing() {
		state, err := s.Cache.FileState(key)
		if err != nil {
			return nil, err
		}
		if state.TooLarge {
			st.tooLarge = true
			return nil, errTooLarge
		}
	}
	opts := figma.FileOptions{Version: s.version, BranchData: true}
	entryKey := cache.Key("file", opts.Query())
	raw, hit, err := s.Cache.Get(key, entryKey, stamp)
	if err != nil {
		return nil, err
	}
	if !hit {
		// Fetching the whole document is speculative: it pays off on a small
		// file and is refused outright on a large one. Give it only part of
		// the budget so a file that will never return whole does not spend
		// the caller's entire timeout before the targeted requests start.
		fetchCtx, cancelFetch := context.WithTimeout(ctx, s.speculativeTimeout())
		raw, err = s.Client.GetFileRaw(fetchCtx, key, opts)
		cancelFetch()
		if err != nil {
			// Fetching the whole document is an optimisation, not a
			// requirement. Figma refuses it outright on large files, and the
			// attempt can outlast the timeout, so degrade to the narrower
			// requests instead of failing the user's command. Auth, missing
			// file, and rate limit errors still surface: a smaller request
			// would not survive them either.
			switch {
			case figma.IsRequestTooLarge(err):
				s.ctx.Log.Warnf("file %s is too large for Figma to return whole; using targeted requests instead", key)
				if cerr := s.Cache.SetTooLarge(key, true); cerr != nil {
					s.ctx.Log.Warnf("cache write failed: %v", cerr)
				}
				st.tooLarge = true
				return nil, errTooLarge
			case figctl.From(err).Code == figctl.CodeNetwork:
				// Transient, so this is not recorded against the file.
				s.ctx.Log.Warnf("fetching the whole document of %s failed (%v); using targeted requests instead", key, figctl.From(err).Message)
				st.tooLarge = true
				return nil, errTooLarge
			}
			return nil, err
		}
		if limit := int64(s.ctx.Flags.MaxFileMB) * 1024 * 1024; int64(len(raw)) > limit {
			s.ctx.Log.Warnf("file %s is %s, above --max-file-mb %d; it will not be cached whole", key, cache.FormatBytes(int64(len(raw))), s.ctx.Flags.MaxFileMB)
			if err := s.Cache.SetTooLarge(key, true); err != nil {
				s.ctx.Log.Warnf("cache write failed: %v", err)
			}
		} else if err := s.Cache.Put(key, entryKey, stamp, raw); err != nil {
			s.ctx.Log.Warnf("cache write failed: %v", err)
		}
	} else {
		s.ctx.Log.Debugf("cache hit whole file %s", key)
	}
	var head fileHead
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "decoding file document: "+err.Error())
	}
	st.raw = raw
	s.touch(key, head.Name, head.Version, head.LastModified)
	return raw, nil
}

// fileHead is the part of a file response the envelope needs.
type fileHead struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	LastModified string `json:"lastModified"`
}

// Scoped fetches the document narrowed to the given nodes. Figma returns the
// requested subtrees plus every node between them and the root, so the result
// carries the page a node sits on and the ancestors above it.
//
// This is how a file too large to return whole is still read as a document.
// GET nodes returns a subtree with no idea where it sits, which loses the page
// name, the ancestor path, and the measurements a designer pinned on the page.
func (s *Session) Scoped(ctx context.Context, key string, ids []string) (*figma.GetFileResponse, error) {
	if len(ids) == 0 {
		return nil, errTooLarge
	}
	opts := figma.FileOptions{Version: s.version, IDs: ids}
	file, err := cached(ctx, s, key, cache.Key("file", opts.Query()), func(ctx context.Context) (*figma.GetFileResponse, error) {
		return s.Client.GetFile(ctx, key, opts)
	})
	if err != nil {
		return nil, err
	}
	s.touch(key, file.Name, file.Version, file.LastModified)
	return file, nil
}

// Overview returns the file with pages and their top level children
// (depth 2), served from the whole document when available.
func (s *Session) Overview(ctx context.Context, key string) (*figma.GetFileResponse, error) {
	// Reuse the whole document only when it is already in hand. Fetching it
	// just for an outline costs minutes on a large file and can be refused
	// outright, and depth 2 answers the question directly.
	if st := s.state(key); st.full != nil {
		copied := *st.full
		copied.Document = st.full.Document.Prune(2)
		return &copied, nil
	}
	opts := figma.FileOptions{Version: s.version, Depth: 2, BranchData: true}
	file, err := cached(ctx, s, key, cache.Key("file", opts.Query()), func(ctx context.Context) (*figma.GetFileResponse, error) {
		return s.Client.GetFile(ctx, key, opts)
	})
	if err != nil {
		return nil, err
	}
	s.touch(key, file.Name, file.Version, file.LastModified)
	return file, nil
}

// Nodes returns the requested nodes. Without geometry or plugin data it
// serves them from the whole document; otherwise, or when the document is
// too large, it calls GET nodes and caches the result.
func (s *Session) Nodes(ctx context.Context, key string, ids []string, opts figma.NodesOptions) (*figma.GetFileNodesResponse, error) {
	opts.Version = s.version
	if opts.Geometry == "" && opts.PluginData == "" {
		file, err := s.File(ctx, key)
		if err == nil {
			return nodesFromFile(file, ids, opts.Depth), nil
		}
		if !errors.Is(err, errTooLarge) {
			return nil, err
		}
	}
	q := opts.Query()
	q.Set("ids", joinIDs(ids))
	nodes, err := cached(ctx, s, key, cache.Key("nodes", q), func(ctx context.Context) (*figma.GetFileNodesResponse, error) {
		return s.Client.GetFileNodes(ctx, key, ids, opts)
	})
	if err != nil {
		return nil, err
	}
	s.touch(key, nodes.Name, nodes.Version, nodes.LastModified)
	return nodes, nil
}

// nodesFromFile builds a GET nodes response from a whole document. Each
// entry carries the components, component sets, and styles its subtree
// references, as the real endpoint does. Unknown ids map to nil.
func nodesFromFile(file *figma.GetFileResponse, ids []string, depth int) *figma.GetFileNodesResponse {
	out := &figma.GetFileNodesResponse{
		Name:         file.Name,
		Role:         file.Role,
		LastModified: file.LastModified,
		EditorType:   file.EditorType,
		ThumbnailURL: file.ThumbnailURL,
		Version:      file.Version,
		Nodes:        map[string]*figma.NodeEntry{},
	}
	pruneDepth := -1
	if depth > 0 {
		pruneDepth = depth
	}
	for _, id := range ids {
		node := file.Document.Find(id)
		if node == nil {
			out.Nodes[id] = nil
			continue
		}
		entry := &figma.NodeEntry{
			Document:      node.Prune(pruneDepth),
			Components:    map[string]figma.Component{},
			ComponentSets: map[string]figma.ComponentSet{},
			Styles:        map[string]figma.Style{},
			SchemaVersion: file.SchemaVersion,
		}
		node.Walk(func(n *figma.Node, _ *figma.Node, _ int) bool {
			if n.ComponentID != "" {
				if c, ok := file.Components[n.ComponentID]; ok {
					entry.Components[n.ComponentID] = c
					if set, ok := file.ComponentSets[c.ComponentSetID]; ok {
						entry.ComponentSets[c.ComponentSetID] = set
					}
				}
			}
			for _, styleID := range n.Styles {
				if st, ok := file.Styles[styleID]; ok {
					entry.Styles[styleID] = st
				}
			}
			return true
		})
		out.Nodes[id] = entry
	}
	return out
}

// touch records the file in the envelope.
func (s *Session) touch(key, name, version, lastModified string) {
	s.ctx.SetFile(&output.FileInfo{Key: key, Name: name, Version: version, LastModified: lastModified})
}

// Touch records a file that was referenced without a document fetch.
func (s *Session) Touch(key string) {
	if s.ctx.file == nil || s.ctx.file.Key != key {
		s.ctx.SetFile(&output.FileInfo{Key: key})
	}
}

// Context returns a context bounded by --timeout for one API call chain.
func (s *Session) Context() (context.Context, context.CancelFunc) {
	if s.ctx.Flags.Timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), s.ctx.Flags.Timeout+10*time.Second)
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}

// parseNodeFlags converts --node values (1:2, 1-2, or a Figma URL with a
// node-id) into API node ids, falling back to the node-id of the ref.
func parseNodeFlags(values []string, r ref.Ref) ([]string, error) {
	var ids []string
	for _, v := range values {
		id, err := parseNodeArg(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 && r.NodeID != "" {
		ids = append(ids, r.NodeID)
	}
	return ids, nil
}

func parseNodeArg(v string) (string, error) {
	if id, err := ref.ParseNodeID(v); err == nil {
		return id, nil
	}
	if parsed, err := ref.Parse(v); err == nil && parsed.NodeID != "" {
		return parsed.NodeID, nil
	}
	return "", figctl.Newf(figctl.CodeUsage, "invalid node id %q", v).
		WithHint("Pass a node id like 1:2 or 1-2, or a Figma URL containing node-id=1-2.")
}

// cursorParam extracts a pagination parameter from a Figma next_page or
// prev_page link.
func cursorParam(link, param string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Query().Get(param)
}
