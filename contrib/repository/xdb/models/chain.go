package models

import "database/sql"

type ChainDefinition struct {
	ChainNo   string         `json:"chain_no"`
	VersionID int64          `json:"version_id"`
	Name      string         `json:"name"`
	Status    int            `json:"status"`
	ExtParams sql.NullString `json:"extparams"`
	Layout    sql.NullString `json:"layout"`
	Desc      sql.NullString `json:"desc"`
}
