package glowflow

// Collection is the relation between two nodes.
type Collection struct {
	// FromNode is the name of the node from which the relation starts.
	FromNode string
	// ToNode is the name of the node to which the relation ends.
	ToNode string
	// RelationType is the type of the relation between the two nodes. default is "success"
	RelationType string
	// Remark is the remark of the relation between the two nodes.
	Remark string
}
