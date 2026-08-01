package defaults

import (
	_ "embed"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/repository/jsonfile"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/memory"
)

func init() {
	glowflow.DefaultRepository = jsonfile.NewRepository()
	glowflow.DefaultStateStorage = memory.NewStateStorage()
}
