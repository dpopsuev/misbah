package daemon

import "github.com/dpopsuev/misbah/proxy"

// Re-export permission types from proxy/ for backward compatibility
// within the daemon package. These types are canonical in proxy/.
type ResourceType = proxy.ResourceType
type Decision = proxy.Decision
type PermissionRequest = proxy.PermissionRequest
type PermissionResponse = proxy.PermissionResponse

const (
	ResourceNetwork = proxy.ResourceNetwork
	ResourceMCP     = proxy.ResourceMCP
	ResourcePackage = proxy.ResourcePackage

	DecisionDeny    = proxy.DecisionDeny
	DecisionOnce    = proxy.DecisionOnce
	DecisionAlways  = proxy.DecisionAlways
	DecisionUnknown = proxy.DecisionUnknown
)
