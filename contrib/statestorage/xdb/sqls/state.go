package sqls

// SaveInstanceState 原子写入（存在则更新，不存在则插入）流程实例状态。
const SaveInstanceState = `--sql
MERGE dbo.flow_instance_state WITH (HOLDLOCK) AS target
USING (SELECT @{instance_id} AS instance_id) AS source
	ON target.instance_id = source.instance_id
WHEN MATCHED THEN
	UPDATE SET
		chain_id = @{chain_id},
		[version] = @{version},
		[status] = @{status},
		state_data = @{state_data},
		update_time = GETDATE()
WHEN NOT MATCHED THEN
	INSERT (instance_id, chain_id, [version], [status], state_data, create_time, update_time)
	VALUES (@{instance_id}, @{chain_id}, @{version}, @{status}, @{state_data}, GETDATE(), GETDATE());`

const GetInstanceState = `--sql
SELECT
	state.instance_id,
	state.chain_id,
	state.[version] AS [version],
	state.[status] AS [status],
	state.state_data
FROM dbo.flow_instance_state AS state
WHERE state.instance_id = @{instance_id}`

const listInstanceStates = `--sql
SELECT
	state.instance_id,
	state.chain_id,
	state.[version] AS [version],
	state.[status] AS [status],
	state.state_data
FROM dbo.flow_instance_state AS state
WHERE 1 = 1
	&{state.chain_id}
	&{state.version}
	&{in state.status}
ORDER BY state.instance_id
OFFSET @{offset} ROWS`

// ListInstanceStates 按条件查询实例状态，仅跳过 offset 行，不限制条数。
const ListInstanceStates = listInstanceStates

// ListInstanceStatesWithLimit 按条件分页查询实例状态。
const ListInstanceStatesWithLimit = listInstanceStates + `
FETCH NEXT @{limit} ROWS ONLY`

const DeleteInstanceState = `--sql
DELETE FROM dbo.flow_instance_state
WHERE instance_id = @{instance_id}`

// SaveNodeState 原子写入（存在则更新，不存在则插入）节点状态。
const SaveNodeState = `--sql
MERGE dbo.flow_node_state WITH (HOLDLOCK) AS target
USING (SELECT @{instance_id} AS instance_id, @{node_id} AS node_id) AS source
	ON target.instance_id = source.instance_id
	AND target.node_id = source.node_id
WHEN MATCHED THEN
	UPDATE SET
		chain_id = @{chain_id},
		[version] = @{version},
		[status] = @{status},
		attempts = @{attempts},
		state_data = @{state_data},
		update_time = GETDATE()
WHEN NOT MATCHED THEN
	INSERT (instance_id, node_id, chain_id, [version], [status], attempts, state_data, create_time, update_time)
	VALUES (@{instance_id}, @{node_id}, @{chain_id}, @{version}, @{status}, @{attempts}, @{state_data}, GETDATE(), GETDATE());`

const GetNodeState = `--sql
SELECT
	state.instance_id,
	state.node_id,
	state.chain_id,
	state.[version] AS [version],
	state.[status] AS [status],
	state.attempts,
	state.state_data
FROM dbo.flow_node_state AS state
WHERE state.instance_id = @{instance_id}
	AND state.node_id = @{node_id}`

const listNodeStates = `--sql
SELECT
	state.instance_id,
	state.node_id,
	state.chain_id,
	state.[version] AS [version],
	state.[status] AS [status],
	state.attempts,
	state.state_data
FROM dbo.flow_node_state AS state
WHERE 1 = 1
	&{state.instance_id}
	&{state.chain_id}
	&{like %state.node_id%}
	&{in state.status}
ORDER BY state.instance_id, state.node_id
OFFSET @{offset} ROWS`

// ListNodeStates 按条件查询节点状态，仅跳过 offset 行，不限制条数。
const ListNodeStates = listNodeStates

// ListNodeStatesWithLimit 按条件分页查询节点状态。
const ListNodeStatesWithLimit = listNodeStates + `
FETCH NEXT @{limit} ROWS ONLY`

const DeleteNodeState = `--sql
DELETE FROM dbo.flow_node_state
WHERE instance_id = @{instance_id}
	AND node_id = @{node_id}`
