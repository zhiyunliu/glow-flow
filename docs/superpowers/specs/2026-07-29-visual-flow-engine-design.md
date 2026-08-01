# 可视化流程编辑系统设计说明

## 1. 目标

构建一个面向 Go 的流程编排与执行引擎，满足以下目标：

- 支持通过配置文件或 JSON 描述流程图。
- 每个节点通过注册方式接入引擎。
- 节点之间通过执行结果中的数据和关系类型进行协作。
- 支持节点能力与并发量控制。
- 运行时可编译、验证、执行、暂停和停止流程。
- 保留足够的扩展性，后续可接入可视化编辑器、状态存储和分布式调度。

## 2. 设计原则

1. 以“配置驱动 + 插件注册”为核心。
2. 把“流程定义”和“节点执行逻辑”解耦。
3. 运行时通过调度器传递数据，避免节点之间直接耦合。
4. 默认优先支持单进程内存执行，后续可插入持久化状态存储。
5. 采用显式接口和最小抽象，方便未来扩展为分布式/并发执行。

## 3. 适配当前仓库的结构

当前仓库已经具备基础骨架：

- Engine：流程引擎入口。
- Node：节点抽象。
- Context：运行上下文。
- options：引擎配置。
- 节点类型：使用 string 标识，配置、注册表和节点接口保持一致。

建议在此基础上扩展为以下分层：

```text
glowflow/
  engine.go           // 引擎生命周期
  registry.go         // 节点注册中心
  define.go           // 流程定义与配置结构
  node.go             // 节点接口与编译结果
    dispatcher.go       // 调度、关系路由、数据分发
  worker.go           // 执行工作单元
  context.go          // 运行上下文
  options.go          // 引擎配置项
  repository.go       // 状态/数据访问抽象
  state.storeage.go   // 状态存储抽象
  logger.go           // 日志接口
  chain.go            // 链路编排
  collection.go       // 节点集合/拓扑集合
  defaults.go         // 默认配置
  ...

internal/
  config/            // 配置解析与校验
    runtime/           // 执行器、调度器、数据总线
  registry/          // 插件注册与工厂
  store/             // 状态存储与持久化接口实现
```

## 4. 核心概念

### 4.1 FlowDefinition

流程定义是可视化编辑器导出的配置结构，包含：

- metadata：流程元信息（id、name、root、disabled、layout、extparams）
- endpoints：流程的入/出端点
- nodes：流程中的节点集合
- connections：节点之间的连线集合

建议将配置结构拆分为：

```go
type FlowDefinition struct {
    ID string `json:"id"`
    Version string `json:"version"`
    Metadata FlowMetadata `json:"metadata"`
    Endpoints []Endpoint `json:"endpoints"`
    Nodes []NodeDefinition `json:"nodes"`
    Connections []ConnectionDefinition `json:"connections"`
}
```

### 4.2 NodeDefinition

每个节点定义包含：

- id：节点唯一 ID
- type：节点类型标识
- name：展示名称
- layout：图形布局（x/y/宽高/说明）
- extparams：节点扩展参数

```go
type NodeDefinition struct {
    ID string `json:"id"`
    Type string `json:"type"`
    Name string `json:"name"`
    Layout Layout `json:"layout"`
    ExtParams map[string]any `json:"extparams"`
}
```

### 4.3 ConnectionDefinition

连线定义表示节点之间的依赖或条件流转：

```go
type ConnectionDefinition struct {
    FromID string `json:"fromId"`
    ToID string `json:"toId"`
    Type string `json:"type"`
}
```

其中 Type 可用于表示：

- True / False：条件分支
- case1 / case2：多分支
- default：默认分支
- default：普通连线

## 5. 节点注册机制

### 5.1 注册接口

节点通过工厂函数注册到引擎。节点类型统一使用 `string`，避免在配置、注册表、节点接口之间反复做类型转换：

```go
type NodeFactory func() Node

type NodeDescriptor struct {
    Type string
    Factory NodeFactory
    DefaultConfig any
}
```

### 5.2 注册中心

引擎内部维护一个注册表：

```go
type Registry interface {
    Register(desc NodeDescriptor) error
    Get(nodeType string) (NodeDescriptor, bool)
    List() []NodeDescriptor
}
```

注册时，类型名与节点类型保持一致，例如：

- dbClient
- log
- http
- redis
- condition

## 6. 运行时模型

### 6.1 编译阶段

流程定义加载后，需要通过独立编译器完成编译。编译器依赖流程定义和节点注册中心，不把编译逻辑放进 Engine，也不塞进 FlowDefinition 这种纯配置结构：

1. 解析流程 JSON。
2. 校验节点类型是否存在。
3. 校验连接是否指向有效节点。
4. 构建节点拓扑图。
5. 生成可执行的编译结果。

```go
type FlowCompiler interface {
    Compile(def FlowDefinition, registry Registry) (*CompiledFlow, error)
}
```

```go
type CompiledFlow struct {
    Definition FlowDefinition
    Nodes map[string]CompiledNode
    Graph map[string]map[string][]CompiledNode
}
```

其中 `Graph` 的第一层 key 是节点 ID，第二层 key 是连接关系类型，例如 `True`、`False`、`case1` 或 `default`。

### 6.2 执行阶段

执行阶段由调度器驱动：

- 从起始节点开始。
- 读取输入数据。
- 调用节点执行逻辑。
- 根据 `ExecuteResult.RelationType` 选择下一跳节点。
- 将 `ExecuteResult.Data` 作为下一跳输入继续传递。

## 7. 节点接口设计

第一版建议直接沿用当前项目已经存在的 `Node` 与 `CompiledNode`，避免在设计阶段引入另一套并行接口：

```go
type Node interface {
    Id() string
    Name() string
    Type() string
    IsStartNode() bool
    Config() any
    Position() Position
}

type CompiledNode interface {
    Node
    Execute(ctx Context, data any) (ExecuteResult, error)
    NextNodes() []CompiledNode
}
```

当前 `NextNodes()` 可以先作为无条件后继集合使用。编译器内部仍应保存基于 `ConnectionDefinition.Type` 的关系图，用于在调度时按 `ExecuteResult.RelationType` 选择下一跳。后续实现时可以把 `NextNodes()` 扩展为按关系类型查询，或者在 `CompiledFlow` 中集中维护关系路由。

### 7.1 执行结果

```go
type ExecuteResult struct {
    RelationType string
    Data any
    Error error
}
```

其中：

- RelationType 表示当前节点执行后选择的关系类型，和 ConnectionDefinition.Type 对齐。
- Data 表示传递给下一跳节点的业务数据。
- Error 表示节点执行中的业务错误。

## 8. 数据传递机制

第一版不强行引入独立 Message 对象，先沿用当前代码中的 `data any` 和 `ExecuteResult.Data`：

```go
Execute(ctx Context, data any) (ExecuteResult, error)
```

设计说明：

- 节点不直接指定 From / To，路由由编译后的拓扑和 `RelationType` 决定。
- `Context` 继续作为运行上下文，承载取消、超时、运行期变量等信息。
- `data any` 只承载业务输入输出，保持节点接口简单。
- 如果后续需要 trace、metadata、消息 ID 或分布式队列，再将 `data any` 演进为 Message 对象。

## 9. 并发与能力控制

### 9.1 节点并发控制

每个节点都应具备执行能力控制：

```go
type CapabilityConfig struct {
    Concurrency int `json:"concurrency"`
    Timeout int `json:"timeout"`
    Retry int `json:"retry"`
}
```

设计建议：

- concurrency 表示同一节点允许同时执行的最大实例数。
- timeout 表示单次执行超时时间。
- retry 表示失败后的重试次数。

### 9.2 调度器策略

调度器维护一个工作池：

- 同一节点的并发数受配置控制。
- 若节点超出并发上限，则待处理数据进入等待队列。
- 通过信号量或通道实现资源控制。

## 10. 引擎生命周期

引擎应支持同时管理多个流程定义，并且每个流程定义都可以独立运行、升级、暂停和停止。建议将职责明确划分为：

- Engine：负责加载流程集合、统一调度、管理运行实例与生命周期。
- FlowDefinition：保持为纯配置结构，承载流程 ID、版本、节点和连线定义。
- FlowCompiler：负责把 FlowDefinition 编译为 CompiledFlow。

```go
func (e *Engine) Load(defs ...FlowDefinition) error
func (e *Engine) Run() error
func (e *Engine) Stop() error
func (e *Engine) Pause(flowID string) error
func (e *Engine) Resume(flowID string) error
func (e *Engine) Reload(def FlowDefinition) error
```

### 10.1 运行流程

1. Load：将一组 FlowDefinition 一次性注册到引擎。
2. Compile：由 FlowCompiler 完成编译，生成可执行拓扑。
3. Run：引擎启动已加载流程集合的执行。
4. Stop：停止引擎上所有已启动的流程实例。
5. Pause/Resume：按 flowID 对指定流程进行暂停或恢复，保持对其他流程的影响最小。
6. Reload：用新的 FlowDefinition 替换同一个 flowID 的旧版本，flowID 从 FlowDefinition.ID 读取，不需要额外参数。

### 10.2 热更新策略

热更新的目标是“新配置生效，旧实例继续运行”。建议采用以下规则：

- 以 FlowDefinition.ID 作为流程唯一标识。
- 同一个 flowID 的新版本配置会被视为新的“定义版本”。
- 已经启动的运行实例继续使用旧版本编译结果。
- 新的调度请求使用最新版本定义。
- 通过版本号或 revision 字段区分定义版本，避免冲突。

```go
type FlowVersion struct {
    FlowID string
    Version string
    Definition FlowDefinition
    Compiled *CompiledFlow
}
```

这样做的好处是：

- 旧流程实例可以稳定结束。
- 新流程请求可以立即使用最新版本。
- 运行时不需要中断已有任务。

## 11. 配置与运行场景

### 11.1 配置场景

当前给出的 JSON 示例可以直接映射成：

- metadata：流程元信息
- nodes：节点定义
- connections：连线定义

其中：

- dbClient 节点代表数据库访问节点。
- log 节点代表日志节点。
- 两个节点之间有三条连接，分别表示不同条件分支。

### 11.2 运行场景

适合的执行行为为：

- 从数据库节点读取结果。
- 根据返回值决定走 True / False / case1 分支。
- 将执行结果中的数据传递给日志节点做记录。

这类流程非常适合用“节点注册 + 关系路由 + 并发控制”的模型。

## 12. 面向后续扩展的设计

### 12.1 可视化编辑器集成

后续可以将流程定义直接作为前端编辑器的模型，支持：

- 节点拖拽
- 连接线绘制
- 布局保存
- 配置面板编辑

### 12.2 状态存储

状态存储只负责运行处理过程中的状态，不承担流程定义的管理端存储。第一阶段可提供内存实现；后续可接入：

- Redis
- MySQL
- MongoDB
- Kafka / NATS 等消息中间件

当前运行期状态抽象以项目中的 `StateStorage` 为准，主要覆盖：

- InstanceState：流程运行实例状态。
- NodeState：节点执行状态。

流程定义的新增、删除、版本发布属于管理端能力，不放进 StateStorage。

### 12.3 分布式执行

当节点数量增加时，可将执行器拆分为：

- 调度器
- 执行器 worker
- 数据总线或消息总线

## 13. 推荐的第一版范围

为了控制复杂度，第一版建议只实现以下能力：

- 支持流程定义加载与编译。
- 支持节点注册。
- 支持基于 `data any` 和 `ExecuteResult` 的节点连线执行。
- 支持节点并发配置。
- 支持简单的分支路由。
- 先实现内存模式，后续再接入状态存储。

## 14. 实施建议

建议按下面顺序落地：

1. 完成 FlowDefinition 与节点定义结构。
2. 完成注册中心与节点工厂。
3. 完成编译器，将配置图转换成执行拓扑。
4. 完成基于 `ExecuteResult.RelationType` 的调度器。
5. 完成节点并发控制与错误处理。
6. 完成测试与简单示例。

## 15. 结论

这套设计将当前仓库中的 Engine、Node、CompiledNode、ExecuteResult、Context、StateStorage 等抽象，扩展为一个“配置驱动、节点注册、关系路由、并发控制”的流程编排引擎。它既适合当前 JSON 配置结构，也能够自然演进到可视化编辑器、状态存储与分布式执行场景。
