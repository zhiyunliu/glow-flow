package models

import "database/sql"

type NodeDefinition struct {
	NodeID           int64          `json:"node_id"`
	VersionID        int64          `json:"version_id"`
	ChainNo          string         `json:"chain_no"`
	NodeDefNo        string         `json:"node_def_no"`
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	ExtParams        sql.NullString `json:"extparams"`
	Layout           sql.NullString `json:"layout"`
	DefinitionParams sql.NullString `json:"definition_extparams"`
	LayoutDefinition sql.NullString `json:"layout_definition"`
	Desc             sql.NullString `json:"desc"`
}
