package action

/*
http请求调用
使用
obj:=glue.Http("")
res,err:= obj.Request(ctx,service,msg.data)

service支持：http,https,xhttp
*/
type nodeHttpCall struct {
}
