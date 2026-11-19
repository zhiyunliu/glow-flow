package glowflow

import "github.com/zhiyunliu/golibs/xtypes"

type Layout struct {
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
	Status    int            `json:"status"`
	ExtParams map[string]any `json:"extparams,omitempty"`
	Layout    Layout         `json:"layout,omitempty"`
}

// EndpointDefinition is the definition of the endpoint.
type EndpointDefinition struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	ExtParams map[string]any `json:"extparams,omitempty"`
	Desc      string         `json:"desc,omitempty"`
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
	ID         string      `json:"id"`
	NodeType   string      `json:"type"`
	NodeName   string      `json:"name"`
	NodeLayout Layout      `json:"layout"`
	ExtParams  xtypes.XMap `json:"extparams,omitempty"`
}

func (n NodeDefinition) Id() string {
	return n.ID
}

func (n NodeDefinition) Name() string {
	return n.NodeName
}

func (n NodeDefinition) Type() string {
	return n.NodeType
}

func (n NodeDefinition) Config() any {
	return n.ExtParams
}

func (n NodeDefinition) Layout() Layout {
	return n.NodeLayout
}

// ConnectionDefinition is the definition of the connection.
type ConnectionDefinition struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
	Remark string `json:"remark,omitempty"`
}

type BasicInfra struct {
	InfraNo   string         `json:"infra_no"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Desc      string         `json:"desc"`
	ExtParams map[string]any `json:"extparams,omitempty"`
}
