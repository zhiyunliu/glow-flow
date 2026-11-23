package action

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/golibs/xtypes"
)

const helperEnvKey = "GLOWFLOW_ACTION_HELPER"

// TestHelperProcess 被 command 节点作为子进程调用(以当前测试程序作为可执行文件，跨平台)
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvKey) != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(2)
	}
	switch args[1] {
	case "echo-json":
		stdin, _ := io.ReadAll(os.Stdin)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"args": args[2:], "stdin": string(stdin), "env": os.Getenv("ORDER_STATUS"),
		})
	case "text":
		fmt.Println("  hello  ")
	case "fail":
		fmt.Fprint(os.Stderr, "boom")
		os.Exit(3)
	case "sleep":
		time.Sleep(30 * time.Second)
	}
	os.Exit(0)
}

func helperCommand(mode string, extra xtypes.XMap, args ...any) xtypes.XMap {
	ext := xtypes.XMap{
		"command": os.Args[0],
		"args":    append([]any{"-test.run=^TestHelperProcess$", "--", mode}, args...),
		"env":     map[string]any{helperEnvKey: "1", "ORDER_STATUS": "${status}"},
	}
	for k, v := range extra {
		ext[k] = v
	}
	return ext
}

func TestCommandSuccess(t *testing.T) {
	node := buildNode(t, CommandNodeType, helperCommand("echo-json", xtypes.XMap{"stdin": true}, "${order_no}", "a b;rm -rf /"))

	msg := glowflow.NewMessage("msg-1", map[string]any{"order_no": "A1", "status": "paid"})
	assertSuccessData(t, execute(t, node, msg), msg, map[string]any{
		"args":  []any{"A1", "a b;rm -rf /"},
		"stdin": `{"order_no":"A1","status":"paid"}`,
		"env":   "paid",
	})

	text := buildNode(t, CommandNodeType, helperCommand("text", nil))
	assertSuccessData(t, execute(t, text, msg), msg, "hello")
}

func TestCommandFailures(t *testing.T) {
	msg := glowflow.NewMessage("msg-1", map[string]any{"status": "paid"})

	assertFailure(t, execute(t, buildNode(t, CommandNodeType, helperCommand("fail", nil)), msg), msg)

	sleep := buildNode(t, CommandNodeType, helperCommand("sleep", xtypes.XMap{"timeout": "200ms"}))
	start := time.Now()
	assertFailure(t, execute(t, sleep, msg), msg)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}

	missing := buildNode(t, CommandNodeType, xtypes.XMap{"command": "glowflow-command-not-exists"})
	assertFailure(t, execute(t, missing, msg), msg)

	assertInvalidConfig(t, CommandNodeType, xtypes.XMap{})
	assertInvalidConfig(t, CommandNodeType, xtypes.XMap{"command": "x", "env": "not json"})
}
