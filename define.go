package glowflow

type Layout struct {
	Desc string `json:"desc,omitempty"`
	Icon string `json:"icon,omitempty"`
	H    int    `json:"H,omitempty"` // 高度
	X    int    `json:"X,omitempty"` // X轴位置
	Y    int    `json:"Y,omitempty"` // Y轴位置
	W    int    `json:"W,omitempty"` // 宽度
}

// ChainMetadata is the metadata of the chain.
type ChainMetadata struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Root      bool           `json:"root"`
	Disabled  bool           `json:"disabled"`
	ExtParams map[string]any `json:"extparams,omitempty"`
	Layout    Layout         `json:"layout,omitempty"`
}

// EndpointDefinition is the definition of the endpoint.
type EndpointDefinition struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	ExtParams map[string]any `json:"extparams,omitempty"`
}

// ChainDefinition is the definition of the chain.
type ChainDefinition struct {
	ID          string                 `json:"id"`
	Version     string                 `json:"version"`
	Metadata    ChainMetadata          `json:"metadata"`
	Endpoints   []EndpointDefinition   `json:"endpoints"`
	Nodes       []NodeDefinition       `json:"nodes"`
	Connections []ConnectionDefinition `json:"connections"`
}

// NodeDefinition is the definition of the node.
type NodeDefinition struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Layout    Layout         `json:"layout"`
	ExtParams map[string]any `json:"extparams,omitempty"`
}

// ConnectionDefinition is the definition of the connection.
type ConnectionDefinition struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
	Remark string `json:"remark,omitempty"`
}
