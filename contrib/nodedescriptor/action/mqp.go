package action

import (
	"fmt"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/utility"
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/queue"
)

const MsgQueueProducerNodeType = "mqp"

/*
消息队列生产者(节点类型 mqp)
使用
obj:=glue.Queue(queuename)
obj.Send(ctx,queuekey,msg.data)

extparams 配置：
  - queuename：glue 队列配置名称，默认 default
  - queuekey：队列 key，必填，支持 ${} 模板
  - delay：延迟发送时间，如 30s 或秒数；>0 时使用 DelaySend(按秒向上取整)
  - timeout：超时时间，如 5s 或秒数

发送成功走 success 分支并原样传递消息；发送失败走 failure 分支。
*/
type nodeMsgQueueProducer struct {
	glowflow.Node
	queueName string
	queueKey  utility.Template
	delay     time.Duration
	timeout   time.Duration
}

// queueResolver 获取 glue 队列对象，测试时可替换
var queueResolver = func(name string) queue.IQueue {
	return glue.Queue(name)
}

func (n *nodeMsgQueueProducer) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	key, err := renderTemplate(n.queueKey, templateEnv(ctx, msg))
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}
	if key = strings.TrimSpace(key); key == "" {
		return failureResult(ctx, n, msg, fmt.Errorf("queue key is empty"))
	}

	sendCtx, cancel := withTimeout(ctx, n.timeout)
	defer cancel()

	q := queueResolver(n.queueName)
	if n.delay > 0 {
		err = q.DelaySend(sendCtx, key, messageData(msg), delaySeconds(n.delay))
	} else {
		err = q.Send(sendCtx, key, messageData(msg))
	}
	if err != nil {
		return failureResult(ctx, n, msg, fmt.Errorf("send queue %s key %s: %w", n.queueName, key, err))
	}
	return passResult(msg), nil
}

func delaySeconds(d time.Duration) int64 {
	secs := int64(d / time.Second)
	if d%time.Second != 0 {
		secs++
	}
	return secs
}

type msgQueueProducerFactory struct{}

func (f *msgQueueProducerFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	ext := def.ExtParams
	node := &nodeMsgQueueProducer{
		Node:      def,
		queueName: strings.TrimSpace(ext.GetString("queuename")),
	}
	if node.queueName == "" {
		node.queueName = defaultClientName
	}

	queueKey := strings.TrimSpace(ext.GetString("queuekey"))
	if queueKey == "" {
		return nil, invalidConfig(def, "queuekey is empty")
	}
	var err error
	if node.queueKey, err = compileTemplate(def, "queuekey", queueKey); err != nil {
		return nil, err
	}
	if node.delay, err = getDuration(ext, "delay", 0); err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	if node.timeout, err = getDuration(ext, "timeout", 0); err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	return node, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    MsgQueueProducerNodeType,
		Factory: &msgQueueProducerFactory{},
	})
}
