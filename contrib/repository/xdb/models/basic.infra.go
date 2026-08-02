package models

import "database/sql"

type BasicInfra struct {
	InfraNo   string         `json:"infra_no"`
	InfraType string         `json:"infra_type"`
	ExtParams sql.NullString `json:"extparams"`
	Desc      sql.NullString `json:"desc"`
}
