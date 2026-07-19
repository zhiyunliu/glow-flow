package glowflow

type Chain struct {
	Params      ChanParams
	Nodes       []Node
	Connections []Collection
}
type ChanParams struct {
	Name        string // name of the chain
	Version     string // version of the chain
	Category    string // category of the chain
	Description string // description of the chain
}
type CompiledChain struct {
}

type ChainInstance struct {
	InstanceID    string
	CompiledChain CompiledChain
}
