package figma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Token scopes named in AUTH_SCOPE hints.
const (
	ScopeFileContent    = "file_content:read"
	ScopeFileMetadata   = "file_metadata:read"
	ScopeLibraryContent = "library_content:read"
	// ScopeLibraryAssets covers the by-key lookups of individual published
	// components, component sets, and styles.
	ScopeLibraryAssets      = "library_assets:read"
	ScopeTeamLibraryContent = "team_library_content:read"
	ScopeFileVariables      = "file_variables:read"
	ScopeFileVariablesWrite = "file_variables:write"
	ScopeFileDevResources   = "file_dev_resources:read"
	// ScopeFileDevResourcesWrite covers creating, updating, and deleting the
	// Dev Mode links attached to nodes.
	ScopeFileDevResourcesWrite = "file_dev_resources:write"
	ScopeFileCommentsRead      = "file_comments:read"
	ScopeFileCommentsWrite     = "file_comments:write"
	ScopeFileVersions          = "file_versions:read"
	ScopeProjects              = "projects:read"
	ScopeFolders               = "folders:read"
	ScopeCurrentUser           = "current_user:read"
)

// FileOptions are the query parameters of GET /v1/files/:key.
type FileOptions struct {
	Version    string
	IDs        []string
	Depth      int
	Geometry   string
	PluginData string
	BranchData bool
}

// Query returns the options as query parameters.
func (o FileOptions) Query() url.Values {
	q := url.Values{}
	if o.Version != "" {
		q.Set("version", o.Version)
	}
	if len(o.IDs) > 0 {
		q.Set("ids", joinIDs(o.IDs))
	}
	if o.Depth > 0 {
		q.Set("depth", strconv.Itoa(o.Depth))
	}
	if o.Geometry != "" {
		q.Set("geometry", o.Geometry)
	}
	if o.PluginData != "" {
		q.Set("plugin_data", o.PluginData)
	}
	if o.BranchData {
		q.Set("branch_data", "true")
	}
	return q
}

// GetFile fetches a file document.
func (c *Client) GetFile(ctx context.Context, key string, opts FileOptions) (*GetFileResponse, error) {
	var out GetFileResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key),
		query:    opts.Query(),
		scope:    ScopeFileContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFileRaw fetches a file document and returns the response body as
// Figma sent it, for callers that cache the bytes before decoding.
func (c *Client) GetFileRaw(ctx context.Context, key string, opts FileOptions) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key),
		query:    opts.Query(),
		scope:    ScopeFileContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NodesOptions are the query parameters of GET /v1/files/:key/nodes other
// than ids.
type NodesOptions struct {
	Version    string
	Depth      int
	Geometry   string
	PluginData string
}

// Query returns the options as query parameters.
func (o NodesOptions) Query() url.Values {
	q := url.Values{}
	if o.Version != "" {
		q.Set("version", o.Version)
	}
	if o.Depth > 0 {
		q.Set("depth", strconv.Itoa(o.Depth))
	}
	if o.Geometry != "" {
		q.Set("geometry", o.Geometry)
	}
	if o.PluginData != "" {
		q.Set("plugin_data", o.PluginData)
	}
	return q
}

// GetFileNodes fetches the given nodes. Long id lists are split across
// several requests and the results merged.
func (c *Client) GetFileNodes(ctx context.Context, key string, ids []string, opts NodesOptions) (*GetFileNodesResponse, error) {
	path := "/v1/files/" + url.PathEscape(key) + "/nodes"
	base := opts.Query()
	merged := &GetFileNodesResponse{Nodes: map[string]*NodeEntry{}}
	for _, batch := range SplitIDs(ids, c.idBudget(path, base)) {
		q := cloneValues(base)
		q.Set("ids", joinIDs(batch))
		var out GetFileNodesResponse
		err := c.do(ctx, request{
			method:   http.MethodGet,
			path:     path,
			query:    q,
			scope:    ScopeFileContent,
			resource: "file " + key + " (nodes " + joinIDs(batch) + ")",
		}, &out)
		if err != nil {
			return nil, err
		}
		if merged.Name == "" {
			merged.Name, merged.Role, merged.LastModified = out.Name, out.Role, out.LastModified
			merged.EditorType, merged.ThumbnailURL, merged.Version = out.EditorType, out.ThumbnailURL, out.Version
		}
		for id, entry := range out.Nodes {
			merged.Nodes[id] = entry
		}
	}
	return merged, nil
}

// GetFileNodesRaw fetches the given nodes and returns the response as Figma
// sent it, for callers that must not lose unknown properties. Long id lists
// are split across several requests; the nodes objects are merged and the
// other top level fields come from the first response.
func (c *Client) GetFileNodesRaw(ctx context.Context, key string, ids []string, opts NodesOptions) (json.RawMessage, error) {
	path := "/v1/files/" + url.PathEscape(key) + "/nodes"
	base := opts.Query()
	var top map[string]json.RawMessage
	nodes := map[string]json.RawMessage{}
	for _, batch := range SplitIDs(ids, c.idBudget(path, base)) {
		q := cloneValues(base)
		q.Set("ids", joinIDs(batch))
		var out map[string]json.RawMessage
		err := c.do(ctx, request{
			method:   http.MethodGet,
			path:     path,
			query:    q,
			scope:    ScopeFileContent,
			resource: "file " + key + " (nodes " + joinIDs(batch) + ")",
		}, &out)
		if err != nil {
			return nil, err
		}
		if top == nil {
			top = out
		}
		var batchNodes map[string]json.RawMessage
		if raw, ok := out["nodes"]; ok {
			if err := json.Unmarshal(raw, &batchNodes); err != nil {
				return nil, internalErr(err, "decoding nodes response for file "+key)
			}
		}
		for id, entry := range batchNodes {
			nodes[id] = entry
		}
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	merged, err := json.Marshal(nodes)
	if err != nil {
		return nil, internalErr(err, "encoding nodes response for file "+key)
	}
	top["nodes"] = merged
	raw, err := json.Marshal(top)
	if err != nil {
		return nil, internalErr(err, "encoding nodes response for file "+key)
	}
	return raw, nil
}

// GetFileMeta fetches file metadata (Tier 3, cheap).
func (c *Client) GetFileMeta(ctx context.Context, key string) (*GetFileMetaResponse, error) {
	var out GetFileMetaResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/meta",
		scope:    ScopeFileMetadata,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ImageOptions are the query parameters of GET /v1/images/:key other than
// ids. Boolean pointers are sent only when set so Figma's defaults apply.
type ImageOptions struct {
	Version           string
	Scale             float64
	Format            string
	SVGOutlineText    *bool
	SVGIncludeID      *bool
	SVGIncludeNodeID  *bool
	SVGSimplifyStroke *bool
	ContentsOnly      *bool
	UseAbsoluteBounds *bool
}

// Query returns the options as query parameters.
func (o ImageOptions) Query() url.Values {
	q := url.Values{}
	if o.Version != "" {
		q.Set("version", o.Version)
	}
	if o.Scale > 0 {
		q.Set("scale", strconv.FormatFloat(o.Scale, 'f', -1, 64))
	}
	if o.Format != "" {
		q.Set("format", o.Format)
	}
	setBool := func(name string, v *bool) {
		if v != nil {
			q.Set(name, strconv.FormatBool(*v))
		}
	}
	setBool("svg_outline_text", o.SVGOutlineText)
	setBool("svg_include_id", o.SVGIncludeID)
	setBool("svg_include_node_id", o.SVGIncludeNodeID)
	setBool("svg_simplify_stroke", o.SVGSimplifyStroke)
	setBool("contents_only", o.ContentsOnly)
	setBool("use_absolute_bounds", o.UseAbsoluteBounds)
	return q
}

// GetImages renders nodes and returns temporary URLs. Long id lists are
// split across several requests and the results merged.
func (c *Client) GetImages(ctx context.Context, key string, ids []string, opts ImageOptions) (*GetImagesResponse, error) {
	path := "/v1/images/" + url.PathEscape(key)
	base := opts.Query()
	merged := &GetImagesResponse{Images: map[string]*string{}}
	for _, batch := range SplitIDs(ids, c.idBudget(path, base)) {
		q := cloneValues(base)
		q.Set("ids", joinIDs(batch))
		var out GetImagesResponse
		err := c.do(ctx, request{
			method:   http.MethodGet,
			path:     path,
			query:    q,
			scope:    ScopeFileContent,
			resource: "file " + key,
		}, &out)
		if err != nil {
			return nil, err
		}
		if out.Err != nil && merged.Err == nil {
			merged.Err = out.Err
		}
		for id, u := range out.Images {
			merged.Images[id] = u
		}
	}
	return merged, nil
}

// GetImageFills returns download URLs for every raster image fill keyed
// by imageRef.
func (c *Client) GetImageFills(ctx context.Context, key string) (*GetImageFillsResponse, error) {
	var out GetImageFillsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/images",
		scope:    ScopeFileContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLocalVariables fetches local variables (Enterprise only).
func (c *Client) GetLocalVariables(ctx context.Context, key string) (*GetLocalVariablesResponse, error) {
	var out GetLocalVariablesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/variables/local",
		scope:    ScopeFileVariables,
		resource: "file " + key,
		plan:     true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPublishedVariables fetches published variables (Enterprise only).
func (c *Client) GetPublishedVariables(ctx context.Context, key string) (*GetPublishedVariablesResponse, error) {
	var out GetPublishedVariablesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/variables/published",
		scope:    ScopeFileVariables,
		resource: "file " + key,
		plan:     true,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PageOptions paginate the team library endpoints and versions.
type PageOptions struct {
	PageSize int
	After    string
	Before   string
}

// Query returns the options as query parameters.
func (o PageOptions) Query() url.Values {
	q := url.Values{}
	if o.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(o.PageSize))
	}
	if o.After != "" {
		q.Set("after", o.After)
	}
	if o.Before != "" {
		q.Set("before", o.Before)
	}
	return q
}

// GetFileComponents lists the components published from a file.
func (c *Client) GetFileComponents(ctx context.Context, key string) (*ComponentsResponse, error) {
	var out ComponentsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/components",
		scope:    ScopeLibraryContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFileComponentSets lists the component sets published from a file.
func (c *Client) GetFileComponentSets(ctx context.Context, key string) (*ComponentSetsResponse, error) {
	var out ComponentSetsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/component_sets",
		scope:    ScopeLibraryContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFileStyles lists the styles published from a file.
func (c *Client) GetFileStyles(ctx context.Context, key string) (*StylesResponse, error) {
	var out StylesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/styles",
		scope:    ScopeLibraryContent,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeamComponents lists a team's published components.
func (c *Client) GetTeamComponents(ctx context.Context, teamID string, page PageOptions) (*ComponentsResponse, error) {
	var out ComponentsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/teams/" + url.PathEscape(teamID) + "/components",
		query:    page.Query(),
		scope:    ScopeTeamLibraryContent,
		resource: "team " + teamID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeamComponentSets lists a team's published component sets.
func (c *Client) GetTeamComponentSets(ctx context.Context, teamID string, page PageOptions) (*ComponentSetsResponse, error) {
	var out ComponentSetsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/teams/" + url.PathEscape(teamID) + "/component_sets",
		query:    page.Query(),
		scope:    ScopeTeamLibraryContent,
		resource: "team " + teamID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeamStyles lists a team's published styles.
func (c *Client) GetTeamStyles(ctx context.Context, teamID string, page PageOptions) (*StylesResponse, error) {
	var out StylesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/teams/" + url.PathEscape(teamID) + "/styles",
		query:    page.Query(),
		scope:    ScopeTeamLibraryContent,
		resource: "team " + teamID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetComponent fetches a published component by key.
func (c *Client) GetComponent(ctx context.Context, key string) (*ComponentResponse, error) {
	var out ComponentResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/components/" + url.PathEscape(key),
		scope:    ScopeLibraryAssets,
		resource: "component " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetComponentSet fetches a published component set by key.
func (c *Client) GetComponentSet(ctx context.Context, key string) (*ComponentSetResponse, error) {
	var out ComponentSetResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/component_sets/" + url.PathEscape(key),
		scope:    ScopeLibraryAssets,
		resource: "component set " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetStyle fetches a published style by key.
func (c *Client) GetStyle(ctx context.Context, key string) (*StyleResponse, error) {
	var out StyleResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/styles/" + url.PathEscape(key),
		scope:    ScopeLibraryAssets,
		resource: "style " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVersions lists the saved versions of a file.
func (c *Client) GetVersions(ctx context.Context, key string, page PageOptions) (*GetVersionsResponse, error) {
	var out GetVersionsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/versions",
		query:    page.Query(),
		scope:    ScopeFileVersions,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetComments lists the comments in a file. asMarkdown asks Figma to
// return messages as markdown.
func (c *Client) GetComments(ctx context.Context, key string, asMarkdown bool) (*GetCommentsResponse, error) {
	q := url.Values{}
	if asMarkdown {
		q.Set("as_md", "true")
	}
	var out GetCommentsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/comments",
		query:    q,
		scope:    ScopeFileCommentsRead,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PostComment adds a comment to a file.
func (c *Client) PostComment(ctx context.Context, key string, body PostCommentRequest) (*Comment, error) {
	var out Comment
	err := c.do(ctx, request{
		method:   http.MethodPost,
		path:     "/v1/files/" + url.PathEscape(key) + "/comments",
		body:     body,
		scope:    ScopeFileCommentsWrite,
		resource: "file " + key,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDevResources lists dev resources, optionally limited to nodes.
func (c *Client) GetDevResources(ctx context.Context, key string, nodeIDs []string) (*GetDevResourcesResponse, error) {
	q := url.Values{}
	if len(nodeIDs) > 0 {
		q.Set("node_ids", joinIDs(nodeIDs))
	}
	var out GetDevResourcesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/files/" + url.PathEscape(key) + "/dev_resources",
		query:    q,
		scope:    ScopeFileDevResources,
		resource: "dev resources for file " + key,
		notFoundHint: "Figma answers 404 here when the file does not exist, when the token lacks the " +
			ScopeFileDevResources + " scope, or when the file's plan does not expose Dev Mode resources.",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PostDevResources creates dev resource links. The endpoint is bulk and can
// create links across several files in one call. Figma answers 200 even when
// some links are rejected, so callers must read the Errors of the result: a
// node accepts at most 10 links and rejects a URL it already carries.
func (c *Client) PostDevResources(ctx context.Context, resources []NewDevResource) (*PostDevResourcesResponse, error) {
	var out PostDevResourcesResponse
	err := c.do(ctx, request{
		method:   http.MethodPost,
		path:     "/v1/dev_resources",
		body:     PostDevResourcesRequest{DevResources: resources},
		scope:    ScopeFileDevResourcesWrite,
		resource: "dev resources",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PutDevResources updates dev resource links by id. Like PostDevResources it
// is bulk and reports per-link failures in the Errors of a 200 response.
func (c *Client) PutDevResources(ctx context.Context, updates []DevResourceUpdate) (*PutDevResourcesResponse, error) {
	var out PutDevResourcesResponse
	err := c.do(ctx, request{
		method:   http.MethodPut,
		path:     "/v1/dev_resources",
		body:     PutDevResourcesRequest{DevResources: updates},
		scope:    ScopeFileDevResourcesWrite,
		resource: "dev resources",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDevResource removes one dev resource link from a file.
func (c *Client) DeleteDevResource(ctx context.Context, fileKey, id string) error {
	return c.do(ctx, request{
		method:   http.MethodDelete,
		path:     "/v1/files/" + url.PathEscape(fileKey) + "/dev_resources/" + url.PathEscape(id),
		scope:    ScopeFileDevResourcesWrite,
		resource: "dev resource " + id + " in file " + fileKey,
	}, nil)
}

// GetMe returns the authenticated user.
func (c *Client) GetMe(ctx context.Context) (*Me, error) {
	var out Me
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/me",
		scope:    ScopeCurrentUser,
		resource: "current user",
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeamProjects lists a team's projects (v1 API).
func (c *Client) GetTeamProjects(ctx context.Context, teamID string) (*GetTeamProjectsResponse, error) {
	var out GetTeamProjectsResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/teams/" + url.PathEscape(teamID) + "/projects",
		scope:    ScopeProjects,
		resource: "team " + teamID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetProjectFiles lists the files in a project (v1 API).
func (c *Client) GetProjectFiles(ctx context.Context, projectID string, branchData bool) (*GetFilesResponse, error) {
	q := url.Values{}
	if branchData {
		q.Set("branch_data", "true")
	}
	var out GetFilesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v1/projects/" + url.PathEscape(projectID) + "/files",
		query:    q,
		scope:    ScopeProjects,
		resource: "project " + projectID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTeamFolders lists a team's top level folders (v2 API).
func (c *Client) GetTeamFolders(ctx context.Context, teamID string) (*GetFoldersResponse, error) {
	var out GetFoldersResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v2/teams/" + url.PathEscape(teamID) + "/folders",
		scope:    ScopeFolders,
		resource: "team " + teamID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFolderFolders lists the subfolders of a folder (v2 API).
func (c *Client) GetFolderFolders(ctx context.Context, folderID string) (*GetFoldersResponse, error) {
	var out GetFoldersResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v2/folders/" + url.PathEscape(folderID) + "/folders",
		scope:    ScopeFolders,
		resource: "folder " + folderID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFolderFiles lists the files in a folder (v2 API).
func (c *Client) GetFolderFiles(ctx context.Context, folderID string, branchData bool) (*GetFilesResponse, error) {
	q := url.Values{}
	if branchData {
		q.Set("branch_data", "true")
	}
	var out GetFilesResponse
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v2/folders/" + url.PathEscape(folderID) + "/files",
		query:    q,
		scope:    ScopeFolders,
		resource: "folder " + folderID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFolderMeta fetches folder metadata (v2 API).
func (c *Client) GetFolderMeta(ctx context.Context, folderID string) (*FolderMeta, error) {
	var out FolderMeta
	err := c.do(ctx, request{
		method:   http.MethodGet,
		path:     "/v2/folders/" + url.PathEscape(folderID) + "/meta",
		scope:    ScopeFolders,
		resource: "folder " + folderID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v)+1)
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

// enterpriseScopes are the scopes Figma offers only to members of an
// Enterprise organization. They are absent from the token screen entirely on
// other plans, so telling someone to tick a box they cannot see sends them
// looking for something that is not there.
var enterpriseScopes = map[string]bool{
	ScopeFileVariables:      true,
	ScopeFileVariablesWrite: true,
}

// EnterpriseOnlyScope reports whether a scope requires an Enterprise plan.
func EnterpriseOnlyScope(scope string) bool { return enterpriseScopes[scope] }
