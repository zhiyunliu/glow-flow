package glowflow

type Layout struct {
	Desc string `json:"desc,omitempty"`
	Icon string `json:"icon,omitempty"`
	H    int    `json:"H,omitempty"`
	X    int    `json:"X,omitempty"`
	Y    int    `json:"Y,omitempty"`
	W    int    `json:"W,omitempty"`
}

type ChainMetadata struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Root      bool           `json:"root"`
	Disabled  bool           `json:"disabled"`
	ExtParams map[string]any `json:"extparams,omitempty"`
	Layout    Layout         `json:"layout,omitempty"`
}

type EndpointDefinition struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	ExtParams map[string]any `json:"extparams,omitempty"`
}

type ChainDefinition struct {
	ID          string                 `json:"id"`
	Version     string                 `json:"version"`
	Metadata    ChainMetadata          `json:"metadata"`
	Endpoints   []EndpointDefinition   `json:"endpoints"`
	Nodes       []NodeDefinition       `json:"nodes"`
	Connections []ConnectionDefinition `json:"connections"`
}

type NodeDefinition struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Layout    Layout         `json:"layout"`
	ExtParams map[string]any `json:"extparams,omitempty"`
}

type ConnectionDefinition struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
	Remark string `json:"remark,omitempty"`
}
