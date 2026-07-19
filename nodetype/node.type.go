package nodetype

type NodeType string

const (
	StartNode NodeType = "start"
	TaskNode  NodeType = "task"
	EndNode   NodeType = "end"
)
