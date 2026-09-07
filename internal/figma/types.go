// Package figma is a hand-written client for the Figma REST API. It covers
// the subset of endpoints figctl uses, maps HTTP failures to figctl error
// codes, retries rate limits and server errors, and batches long ID lists.
package figma

import "encoding/json"

// Component is a component entry in a file response.
type Component struct {
	Key                string              `json:"key"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	ComponentSetID     string              `json:"componentSetId,omitempty"`
	DocumentationLinks []DocumentationLink `json:"documentationLinks"`
	Remote             bool                `json:"remote"`
}

// ComponentSet is a component set entry in a file response.
type ComponentSet struct {
	Key                string              `json:"key"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	DocumentationLinks []DocumentationLink `json:"documentationLinks,omitempty"`
	Remote             bool                `json:"remote,omitempty"`
}

// DocumentationLink is a link attached to a component.
type DocumentationLink struct {
	URI string `json:"uri"`
}

// Style is a style entry in a file response. Values live on the style's
// node.
type Style struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Remote      bool   `json:"remote"`
	StyleType   string `json:"styleType"`
}

// Branch is a branch of a file.
type Branch struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	LastModified string `json:"last_modified"`
}

// GetFileResponse is the response of GET /v1/files/:key.
type GetFileResponse struct {
	Name          string                  `json:"name"`
	Role          string                  `json:"role"`
	LastModified  string                  `json:"lastModified"`
	EditorType    string                  `json:"editorType"`
	ThumbnailURL  string                  `json:"thumbnailUrl,omitempty"`
	Version       string                  `json:"version"`
	Document      *Node                   `json:"document"`
	Components    map[string]Component    `json:"components"`
	ComponentSets map[string]ComponentSet `json:"componentSets"`
	SchemaVersion int                     `json:"schemaVersion"`
	Styles        map[string]Style        `json:"styles"`
	LinkAccess    string                  `json:"linkAccess,omitempty"`
	MainFileKey   string                  `json:"mainFileKey,omitempty"`
	Branches      []Branch                `json:"branches,omitempty"`
}

// NodeEntry is one requested node in a GET nodes response. Document is
// nil when the node does not exist in the file.
type NodeEntry struct {
	Document      *Node                   `json:"document"`
	Components    map[string]Component    `json:"components"`
	ComponentSets map[string]ComponentSet `json:"componentSets"`
	SchemaVersion int                     `json:"schemaVersion"`
	Styles        map[string]Style        `json:"styles"`
}

// GetFileNodesResponse is the response of GET /v1/files/:key/nodes.
type GetFileNodesResponse struct {
	Name         string                `json:"name"`
	Role         string                `json:"role"`
	LastModified string                `json:"lastModified"`
	EditorType   string                `json:"editorType"`
	ThumbnailURL string                `json:"thumbnailUrl,omitempty"`
	Version      string                `json:"version"`
	Nodes        map[string]*NodeEntry `json:"nodes"`
}

// User is a Figma user.
type User struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	ImgURL string `json:"img_url"`
}

// FileMeta is the file block of GET /v1/files/:key/meta.
type FileMeta struct {
	Name          string `json:"name"`
	FolderName    string `json:"folder_name,omitempty"`
	LastTouchedAt string `json:"last_touched_at"`
	Creator       User   `json:"creator"`
	LastTouchedBy *User  `json:"last_touched_by,omitempty"`
	ThumbnailURL  string `json:"thumbnail_url,omitempty"`
	EditorType    string `json:"editorType"`
	Role          string `json:"role,omitempty"`
	LinkAccess    string `json:"link_access,omitempty"`
	URL           string `json:"url,omitempty"`
	Version       string `json:"version,omitempty"`
}

// GetFileMetaResponse is the response of GET /v1/files/:key/meta.
type GetFileMetaResponse struct {
	File FileMeta `json:"file"`
}

// GetImagesResponse is the response of GET /v1/images/:key. A nil URL
// means the node failed to render.
type GetImagesResponse struct {
	Err    *string            `json:"err"`
	Images map[string]*string `json:"images"`
}

// GetImageFillsResponse is the response of GET /v1/files/:key/images.
type GetImageFillsResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		Images map[string]string `json:"images"`
	} `json:"meta"`
}

// VariableMode is a mode of a variable collection.
type VariableMode struct {
	ModeID       string `json:"modeId"`
	Name         string `json:"name"`
	ParentModeID string `json:"parentModeId,omitempty"`
}

// VariableCollection is a local variable collection.
type VariableCollection struct {
	ID                         string                              `json:"id"`
	Name                       string                              `json:"name"`
	Key                        string                              `json:"key"`
	Modes                      []VariableMode                      `json:"modes"`
	DefaultModeID              string                              `json:"defaultModeId"`
	Remote                     bool                                `json:"remote"`
	HiddenFromPublishing       bool                                `json:"hiddenFromPublishing"`
	VariableIDs                []string                            `json:"variableIds"`
	IsExtension                bool                                `json:"isExtension,omitempty"`
	ParentVariableCollectionID string                              `json:"parentVariableCollectionId,omitempty"`
	RootVariableCollectionID   string                              `json:"rootVariableCollectionId,omitempty"`
	VariableOverrides          map[string]map[string]VariableValue `json:"variableOverrides,omitempty"`
}

// VariableCodeSyntax maps platforms to code names.
type VariableCodeSyntax struct {
	Web     string `json:"WEB,omitempty"`
	Android string `json:"ANDROID,omitempty"`
	IOS     string `json:"iOS,omitempty"`
}

// Variable is a local variable.
type Variable struct {
	ID                   string                   `json:"id"`
	Name                 string                   `json:"name"`
	Key                  string                   `json:"key"`
	VariableCollectionID string                   `json:"variableCollectionId"`
	ResolvedType         string                   `json:"resolvedType"`
	ValuesByMode         map[string]VariableValue `json:"valuesByMode"`
	Remote               bool                     `json:"remote"`
	Description          string                   `json:"description"`
	HiddenFromPublishing bool                     `json:"hiddenFromPublishing"`
	Scopes               []string                 `json:"scopes"`
	CodeSyntax           VariableCodeSyntax       `json:"codeSyntax"`
	DeletedButReferenced bool                     `json:"deletedButReferenced,omitempty"`
}

// VariableValue is a variable value in one mode: a boolean, a number, a
// string, a color, or an alias to another variable. Exactly one of the
// fields is set.
type VariableValue struct {
	Bool   *bool
	Number *float64
	String *string
	Color  *Color
	Alias  *VariableAlias
}

// UnmarshalJSON implements json.Unmarshaler.
func (v *VariableValue) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch val := raw.(type) {
	case bool:
		v.Bool = &val
	case float64:
		v.Number = &val
	case string:
		v.String = &val
	case map[string]any:
		if val["type"] == "VARIABLE_ALIAS" {
			var alias VariableAlias
			if err := json.Unmarshal(data, &alias); err != nil {
				return err
			}
			v.Alias = &alias
			return nil
		}
		var color Color
		if err := json.Unmarshal(data, &color); err != nil {
			return err
		}
		v.Color = &color
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (v VariableValue) MarshalJSON() ([]byte, error) {
	switch {
	case v.Bool != nil:
		return json.Marshal(*v.Bool)
	case v.Number != nil:
		return json.Marshal(*v.Number)
	case v.String != nil:
		return json.Marshal(*v.String)
	case v.Color != nil:
		return json.Marshal(v.Color)
	case v.Alias != nil:
		return json.Marshal(v.Alias)
	}
	return []byte("null"), nil
}

// GetLocalVariablesResponse is the response of GET
// /v1/files/:key/variables/local.
type GetLocalVariablesResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		Variables           map[string]Variable           `json:"variables"`
		VariableCollections map[string]VariableCollection `json:"variableCollections"`
	} `json:"meta"`
}

// PublishedVariableCollection is a published variable collection.
type PublishedVariableCollection struct {
	ID           string `json:"id"`
	SubscribedID string `json:"subscribed_id"`
	Name         string `json:"name"`
	Key          string `json:"key"`
	UpdatedAt    string `json:"updatedAt"`
}

// PublishedVariable is a published variable.
type PublishedVariable struct {
	ID                   string `json:"id"`
	SubscribedID         string `json:"subscribed_id"`
	Name                 string `json:"name"`
	Key                  string `json:"key"`
	VariableCollectionID string `json:"variableCollectionId"`
	ResolvedDataType     string `json:"resolvedDataType"`
	UpdatedAt            string `json:"updatedAt"`
}

// GetPublishedVariablesResponse is the response of GET
// /v1/files/:key/variables/published.
type GetPublishedVariablesResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		Variables           map[string]PublishedVariable           `json:"variables"`
		VariableCollections map[string]PublishedVariableCollection `json:"variableCollections"`
	} `json:"meta"`
}

// FrameInfo locates a published component or style in its file.
type FrameInfo struct {
	NodeID                 string   `json:"nodeId,omitempty"`
	Name                   string   `json:"name,omitempty"`
	BackgroundColor        string   `json:"backgroundColor,omitempty"`
	PageID                 string   `json:"pageId"`
	PageName               string   `json:"pageName"`
	ContainingStateGroup   *NodeRef `json:"containingStateGroup,omitempty"`
	ContainingComponentSet *NodeRef `json:"containingComponentSet,omitempty"`
}

// NodeRef names a node by ID.
type NodeRef struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// PublishedComponent is a component from the library endpoints.
type PublishedComponent struct {
	Key             string     `json:"key"`
	FileKey         string     `json:"file_key"`
	NodeID          string     `json:"node_id"`
	ThumbnailURL    string     `json:"thumbnail_url,omitempty"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
	User            User       `json:"user"`
	ContainingFrame *FrameInfo `json:"containing_frame,omitempty"`
}

// PublishedComponentSet is a component set from the library endpoints.
type PublishedComponentSet = PublishedComponent

// PublishedStyle is a style from the library endpoints.
type PublishedStyle struct {
	Key          string `json:"key"`
	FileKey      string `json:"file_key"`
	NodeID       string `json:"node_id"`
	StyleType    string `json:"style_type"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	User         User   `json:"user"`
	SortPosition string `json:"sort_position"`
}

// Cursor is the cursor block of paginated library responses.
type Cursor struct {
	Before *int `json:"before,omitempty"`
	After  *int `json:"after,omitempty"`
}

// ComponentsResponse is the response of the file and team component
// endpoints.
type ComponentsResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		Components []PublishedComponent `json:"components"`
		Cursor     *Cursor              `json:"cursor,omitempty"`
	} `json:"meta"`
}

// ComponentSetsResponse is the response of the file and team component set
// endpoints.
type ComponentSetsResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		ComponentSets []PublishedComponentSet `json:"component_sets"`
		Cursor        *Cursor                 `json:"cursor,omitempty"`
	} `json:"meta"`
}

// StylesResponse is the response of the file and team style endpoints.
type StylesResponse struct {
	Error  bool `json:"error"`
	Status int  `json:"status"`
	Meta   struct {
		Styles []PublishedStyle `json:"styles"`
		Cursor *Cursor          `json:"cursor,omitempty"`
	} `json:"meta"`
}

// ComponentResponse is the response of GET /v1/components/:key.
type ComponentResponse struct {
	Error  bool               `json:"error"`
	Status int                `json:"status"`
	Meta   PublishedComponent `json:"meta"`
}

// ComponentSetResponse is the response of GET /v1/component_sets/:key.
type ComponentSetResponse struct {
	Error  bool                  `json:"error"`
	Status int                   `json:"status"`
	Meta   PublishedComponentSet `json:"meta"`
}

// StyleResponse is the response of GET /v1/styles/:key.
type StyleResponse struct {
	Error  bool           `json:"error"`
	Status int            `json:"status"`
	Meta   PublishedStyle `json:"meta"`
}

// Version is a saved version of a file.
type Version struct {
	ID           string  `json:"id"`
	CreatedAt    string  `json:"created_at"`
	Label        *string `json:"label"`
	Description  *string `json:"description"`
	User         User    `json:"user"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
}

// Pagination holds the page links of a paginated response.
type Pagination struct {
	PrevPage string `json:"prev_page,omitempty"`
	NextPage string `json:"next_page,omitempty"`
}

// GetVersionsResponse is the response of GET /v1/files/:key/versions.
type GetVersionsResponse struct {
	Versions   []Version  `json:"versions"`
	Pagination Pagination `json:"pagination"`
}

// CommentPin locates a comment: absolute canvas coordinates (X, Y), an
// offset inside a node (NodeID, NodeOffset), and optionally a region.
type CommentPin struct {
	X                *float64 `json:"x,omitempty"`
	Y                *float64 `json:"y,omitempty"`
	NodeID           string   `json:"node_id,omitempty"`
	NodeOffset       *Vector  `json:"node_offset,omitempty"`
	RegionHeight     *float64 `json:"region_height,omitempty"`
	RegionWidth      *float64 `json:"region_width,omitempty"`
	CommentPinCorner string   `json:"comment_pin_corner,omitempty"`
}

// Reaction is an emoji reaction on a comment.
type Reaction struct {
	User      User   `json:"user"`
	Emoji     string `json:"emoji"`
	CreatedAt string `json:"created_at"`
}

// Comment is a comment or reply in a file.
type Comment struct {
	ID         string      `json:"id"`
	ClientMeta *CommentPin `json:"client_meta"`
	FileKey    string      `json:"file_key"`
	ParentID   string      `json:"parent_id,omitempty"`
	User       User        `json:"user"`
	CreatedAt  string      `json:"created_at"`
	ResolvedAt *string     `json:"resolved_at"`
	Message    string      `json:"message"`
	OrderID    *string     `json:"order_id"`
	Reactions  []Reaction  `json:"reactions"`
}

// GetCommentsResponse is the response of GET /v1/files/:key/comments.
type GetCommentsResponse struct {
	Comments []Comment `json:"comments"`
}

// PostCommentRequest is the body of POST /v1/files/:key/comments.
type PostCommentRequest struct {
	Message    string      `json:"message"`
	CommentID  string      `json:"comment_id,omitempty"`
	ClientMeta *CommentPin `json:"client_meta,omitempty"`
}

// DevResource is a Dev Mode resource link on a node.
type DevResource struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	FileKey string `json:"file_key"`
	NodeID  string `json:"node_id"`
}

// GetDevResourcesResponse is the response of GET
// /v1/files/:key/dev_resources.
type GetDevResourcesResponse struct {
	DevResources []DevResource `json:"dev_resources"`
}

// NewDevResource is one link to attach to a node, for POST /v1/dev_resources.
// Every field is required by the API.
type NewDevResource struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	FileKey string `json:"file_key"`
	NodeID  string `json:"node_id"`
}

// DevResourceUpdate changes an existing link, for PUT /v1/dev_resources. Only
// the id is required; a blank name or url leaves that field unchanged.
type DevResourceUpdate struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// PostDevResourcesRequest is the body of POST /v1/dev_resources.
type PostDevResourcesRequest struct {
	DevResources []NewDevResource `json:"dev_resources"`
}

// PutDevResourcesRequest is the body of PUT /v1/dev_resources.
type PutDevResourcesRequest struct {
	DevResources []DevResourceUpdate `json:"dev_resources"`
}

// DevResourceError explains why one link in a bulk write was rejected. Figma
// answers 200 even when some links fail, so these must be surfaced.
type DevResourceError struct {
	FileKey string `json:"file_key,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	ID      string `json:"id,omitempty"`
	Error   string `json:"error"`
}

// PostDevResourcesResponse is the response of POST /v1/dev_resources.
type PostDevResourcesResponse struct {
	LinksCreated []DevResource      `json:"links_created"`
	Errors       []DevResourceError `json:"errors,omitempty"`
}

// PutDevResourcesResponse is the response of PUT /v1/dev_resources.
type PutDevResourcesResponse struct {
	LinksUpdated []DevResource      `json:"links_updated"`
	Errors       []DevResourceError `json:"errors,omitempty"`
}

// Me is the response of GET /v1/me.
type Me struct {
	User
	Email string `json:"email"`
}

// Project is a project in a team.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetTeamProjectsResponse is the response of GET /v1/teams/:id/projects.
type GetTeamProjectsResponse struct {
	Name     string    `json:"name"`
	Projects []Project `json:"projects"`
}

// FileSummary is a file listed by the project and folder endpoints.
type FileSummary struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"`
	LastModified string   `json:"last_modified"`
	Branches     []Branch `json:"branches,omitempty"`
}

// GetFilesResponse is the response of the project and folder file
// endpoints.
type GetFilesResponse struct {
	Name  string        `json:"name"`
	Files []FileSummary `json:"files"`
}

// Folder is a folder in a team.
type Folder struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ParentFolderID *string `json:"parent_folder_id"`
}

// GetFoldersResponse is the response of the team and folder subfolder
// endpoints.
type GetFoldersResponse struct {
	Name    string   `json:"name"`
	Folders []Folder `json:"folders"`
}

// FolderMeta is the response of GET /v2/folders/:id/meta and GET
// /v1/projects/:id/meta.
type FolderMeta struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	ThumbnailURL *string `json:"thumbnail_url"`
	FileCount    int     `json:"file_count"`
	UpdatedAt    string  `json:"updated_at"`
	CreatedAt    string  `json:"created_at"`
}
