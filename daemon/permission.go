package daemon

// ResourceType identifies what kind of resource is being requested.
type ResourceType string

const (
	ResourceNetwork ResourceType = "network"
	ResourceMCP     ResourceType = "mcp"
	ResourcePackage ResourceType = "package"
)

// Decision represents the user's permission response.
type Decision string

const (
	DecisionDeny    Decision = "deny"
	DecisionOnce    Decision = "once"
	DecisionAlways  Decision = "always"
	DecisionUnknown Decision = "unknown"
)

// PermissionRequest is sent by a proxy when it encounters an unknown resource.
type PermissionRequest struct {
	Container    string       `json:"container"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   string       `json:"resource_id"`
	Description  string       `json:"description"`
}

// PermissionResponse is the daemon's answer.
type PermissionResponse struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}
