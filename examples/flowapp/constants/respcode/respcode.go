package respcode

// 统一响应码：成功为 0；错误码前三位与 HTTP 状态码一致
const (
	Success     = 0
	BadRequest  = 400001 // 参数错误
	NotFound    = 404001 // 数据不存在
	SystemError = 500001 // 系统错误
	Unavailable = 503001 // 服务暂不可用
)
