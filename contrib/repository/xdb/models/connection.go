package models

import "database/sql"

type ConnectionDefinition struct {
	ConnID    int64          `json:"conn_id"`
	VersionID int64          `json:"version_id"`
	ChainNo   string         `json:"chain_no"`
	FromID    string         `json:"from_id"`
	ToID      string         `json:"to_id"`
	Type      string         `json:"type"`
	Remark    sql.NullString `json:"remark"`
	ExtParams sql.NullString `json:"extparams"`
}
