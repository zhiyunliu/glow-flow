package action

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/utility"
)

const CommandNodeType = "command"

// commandWaitDelay 进程退出(或被终止)后，等待其子进程释放输出管道的最长时间
const commandWaitDelay = time.Second

/*
执行本机命令行(节点类型 command)

命令直接通过 os/exec 执行，不经过 shell 解释，模板变量中的特殊字符不会被当作 shell 语法；
如需管道、重定向等 shell 语法，可配置 command=sh、args=["-c","..."]，此时需自行保证参数安全。

extparams 配置：
  - command：可执行文件路径或名称，必填
  - args：参数列表(数组)，每一项支持 ${} 模板
  - workdir：工作目录，默认为当前进程工作目录
  - env：额外的环境变量(对象)，值支持 ${} 模板，会追加到当前进程环境变量之后
  - stdin：为 true 时将消息数据写入标准输入(string/[]byte 原样，其他类型 JSON 序列化)
  - timeout：超时时间，如 30s 或秒数；超时后进程会被终止

退出码为 0 时走 success 分支，标准输出(JSON 自动解析，否则为去除首尾空白的字符串)作为下游消息数据；
执行失败或退出码非 0 时走 failure 分支，并原样传递消息。
*/
type nodeCommand struct {
	glowflow.Node
	command string
	args    []utility.Template
	workdir string
	env     map[string]utility.Template
	stdin   bool
	timeout time.Duration
}

func (n *nodeCommand) Execute(ctx glowflow.Context, msg *glowflow.Message) (glowflow.ExecuteResult, error) {
	tplEnv := templateEnv(ctx, msg)
	args := make([]string, 0, len(n.args))
	for _, tpl := range n.args {
		arg, err := renderTemplate(tpl, tplEnv)
		if err != nil {
			return failureResult(ctx, n, msg, err)
		}
		args = append(args, arg)
	}
	env, err := renderMap(n.env, tplEnv)
	if err != nil {
		return failureResult(ctx, n, msg, err)
	}

	runCtx, cancel := withTimeout(ctx, n.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, n.command, args...)
	cmd.Dir = n.workdir
	cmd.WaitDelay = commandWaitDelay
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), envList(env)...)
	}
	if n.stdin {
		payload, err := payloadString(messageData(msg))
		if err != nil {
			return failureResult(ctx, n, msg, err)
		}
		cmd.Stdin = strings.NewReader(payload)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := runCtx.Err(); ctxErr != nil {
			err = fmt.Errorf("%w (%v)", ctxErr, err)
		}
		return failureResult(ctx, n, msg, fmt.Errorf("run command %s: %w, stderr: %s", n.command, err, truncate(stderr.Bytes(), 1024)))
	}
	return successResult(msg, decodeResult(stdout.Bytes())), nil
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	list := make([]string, 0, len(keys))
	for _, k := range keys {
		list = append(list, k+"="+env[k])
	}
	return list
}

type commandFactory struct{}

func (f *commandFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	ext := def.ExtParams
	node := &nodeCommand{
		Node:    def,
		command: strings.TrimSpace(ext.GetString("command")),
		workdir: strings.TrimSpace(ext.GetString("workdir")),
		stdin:   ext.GetBool("stdin"),
	}
	if node.command == "" {
		return nil, invalidConfig(def, "command is empty")
	}

	var err error
	if node.args, err = compileTemplates(def, "args", rawStrings(def, "args")); err != nil {
		return nil, err
	}
	env, err := getStringMap(ext, "env")
	if err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	if node.env, err = compileTemplateMap(def, "env", env); err != nil {
		return nil, err
	}
	if node.timeout, err = getDuration(ext, "timeout", 0); err != nil {
		return nil, invalidConfig(def, "%v", err)
	}
	return node, nil
}

// rawStrings 参数列表按原样读取(保留空字符串，不做逗号拆分)
func rawStrings(def glowflow.NodeDefinition, key string) []string {
	val, ok := def.ExtParams.Get(key)
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, fmt.Sprint(item))
		}
		return items
	default:
		return []string{def.ExtParams.GetString(key)}
	}
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    CommandNodeType,
		Factory: &commandFactory{},
	})
}
