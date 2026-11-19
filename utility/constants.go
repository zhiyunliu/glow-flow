package utility

import "strings"

const (
	// VarPrefix 变量前缀
	VarPrefix = "${"
	// VarSuffix 变量后缀
	VarSuffix = "}"
)

func CheckHasVar(str string) bool {
	return strings.Contains(str, VarPrefix) && strings.Contains(str, VarSuffix)
}
