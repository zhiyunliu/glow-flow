# glow-flow 样例

基于 glue 框架使用 glow-flow 的样例，流程覆盖当前已实现的全部节点(email 除外)。

| 目录 | 说明 | 端口 |
|---|---|---|
| `flowapp` | 样例服务：执行流程(API)、消费下单事件触发履约流程(MQC) | 18080 |
| `mockhttp` | 模拟 http 服务：会员信息、发货短信、钉钉群机器人 Webhook | 18081 |
| `mockrpc` | 模拟 rpc(grpc) 服务：风控评估、库存预占、积分发放 | 18082 |

`go.mod` 独立于 glow-flow 主模块(避免为库引入队列、数据库驱动依赖)，通过 `replace` 指向 `../` 与 `../../glue`。

## flowapp 目录结构

参考 glow-flow-manager 的分层：`services` 处理 HTTP/MQC，`modules` 承载业务，`models` 定义参数与数据结构。

```
main.go                     服务入口：注册服务、加载配置、初始化流程引擎
config/                     业务配置(配置文件 flowapp 节点)
constants/                  响应码、错误子码
models/                     请求参数、执行结果、执行轨迹
modules/
  flow/                     流程引擎：加载流程定义、执行流程、重新加载
  flow/trace/               执行轨迹：节点注册表包装器 + 记录器(随 context 传递)
  flow/nodes/               自定义节点(example.collect)
  order/                    下单事件处理：执行履约流程
services/                   HTTP/MQC 处理器与路由(services.go)
pkg/                        通用组件：统一响应、业务错误
docs/sql/                   数据库脚本
glowflow/                   流程定义(jsonfile 仓储读取工作目录下的 glowflow/*.json)
```

## 流程

流程定义位于 `flowapp/glowflow/*.json`，由 `jsonfile` 仓储加载。

| 流程 | 场景 | 使用的节点 |
|---|---|---|
| `order_create` 订单创建 | 校验 → 计算金额 → 黑名单 → 并行查询风控(rpc)与会员(http) → 汇总 → 按风险分流：高风险钉钉告警并拒单 / 转人工审核 / 发布下单事件并保存订单 | exprfilter、jstransform、luafilter、fork、rpccall、httpcall、join、switch、dingtalk、template、exprtransform、mqp、xdb(firstas)、end、custom |
| `order_fulfill` 订单履约 | 由下单事件触发：逐个商品预占库存(缺货时 break) → 全部预占成功后逐件打包 → 并行发送短信、发放积分、记录打包工位 | fork、for、rpccall、exprfilter、break、join、jsfilter、template、while、jstransform、delay、groupaction、httpcall、command、custom |
| `device_alarm` 设备告警 | 数据校验 → 同时检测高温/高湿/低电量 → 汇总告警 → 钉钉通知并记录告警 | exprfilter、fork、inclusive、template、exprtransform、jstransform、join(数组模式)、dingtalk、xdb(exec)、custom |
| `order_query` 订单查询 | 查询用户最近订单并汇总金额 | xdb(queryas)、exprtransform、template、custom |

`custom` 节点使用 `flowapp/modules/flow/nodes` 中注册的 `example.collect`，将消息数据收集为流程结果。

## 运行

依赖 glow-flow 根目录 `docker-compose.yml` 中的 redis 与 sqlserver：

```bash
docker compose up -d   # 在 glow-flow 根目录执行

# 初始化样例数据库 glowflow_example(可重复执行)
docker cp flowapp/docs/sql/glowflow.example.mssql.sql glow-flow-sqlserver:/tmp/glowflow_example_init.sql
docker exec glow-flow-sqlserver /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P "$MSSQL_SA_PASSWORD" -C -i /tmp/glowflow_example_init.sql
```

分别在三个终端中启动(glue 默认读取工作目录下的 config.json)：

```bash
cd mockhttp && go run . run
cd mockrpc && go run . run
# 数据库密码通过环境变量注入，config.json 中使用 glue 的 {ENV} 占位符
cd flowapp && GLOWFLOW_DB_PASSWORD=<sa 密码> go run . run
```

## 调用

`requests.http` 中列出了各流程各分支的请求，例如：

```bash
curl -X POST http://127.0.0.1:18080/flow/run -H "Content-Type: application/json" \
  -d '{"chain":"device_alarm","data":{"device_id":"DEV-001","temperature":56.5,"humidity":85,"battery":60}}'
```

接口统一返回 `{"code": 0, "message": "成功", "data": ...}`；失败时返回 `{"code", "sub_code", "message"}` 及对应的 HTTP 状态码(如流程不存在为 404)。执行流程的 `data` 包含：

- `results`：流程中 `example.collect` 节点收集的结果
- `trace`：节点执行轨迹(按开始顺序)，含分支关系、循环序号(`loop_index`)、输出数据与错误
- `error`：节点返回 error 或 panic 导致执行中断时的错误

样例服务通过包装节点注册表记录轨迹(`flowapp/modules/flow/trace`)，包装时保留了 join 与循环/节点组依赖的接口。

| 接口 | 说明 |
|---|---|
| `POST /flow/run` | 执行流程：`{"chain": "流程ID", "data": 消息数据}` |
| `GET /flow/list` | 已加载的流程 |
| `POST /flow/reload` | 修改流程定义后重新加载 |

`order_create` 正常下单时，`mqp` 节点将订单发布到 redis 队列 `glowflow.example.order.created`，flowapp 的 mqcserver 消费后执行 `order_fulfill`，结果输出在 flowapp 日志中。队列与处理路径(`/order/created`)的对应关系在 `config.json` 的 `servers.mqcserver.tasks` 中配置。钉钉消息可通过 `GET http://127.0.0.1:18081/robot/messages` 查看。

`config.json` 的 `flowapp` 节点：

| 字段 | 默认值 | 说明 |
|---|---|---|
| `fulfill_chain` | `order_fulfill` | 消费下单事件时执行的流程 |
| `run_timeout` | 30 | 单次流程执行的最长时间(秒) |

## 模拟数据

| 数据 | 效果 |
|---|---|
| 用户 `U1001` / `U2002` / `U3003` | 会员等级 gold(9 折) / silver(95 折) / normal |
| 用户 `U4040` | 会员不存在(httpcall 返回 404，走 failure) |
| 用户 `U7001` 或金额 >= 10000 | 风控 high；金额 >= 3000 为 medium |
| 用户 `U9999`、`U6666` | 黑名单 |
| `X` 开头的商品 | 库存不足 |
