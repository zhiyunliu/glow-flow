package sqls

const LoadChainDefinitions = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	WHERE version.[status] = 1
	GROUP BY version.chain_no
)
SELECT
	chain.chain_no,
	version.version_id,
	version.[name] AS [name],
	version.[status] AS [status],
	version.extparams,
	version.layout,
	version.[desc] AS [desc]
FROM dbo.flow_chain_definition AS chain
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = chain.chain_no
INNER JOIN dbo.flow_chain_version AS version
	ON version.chain_no = latest.chain_no
	AND version.version_id = latest.version_id
WHERE chain.[status] = 1
ORDER BY chain.chain_no`

const LoadChainDefinition = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	WHERE version.[status] = 1
		AND version.chain_no = @{chain_no}
	GROUP BY version.chain_no
)
SELECT
	chain.chain_no,
	version.version_id,
	version.[name] AS [name],
	version.[status] AS [status],
	version.extparams,
	version.layout,
	version.[desc] AS [desc]
FROM dbo.flow_chain_definition AS chain
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = chain.chain_no
INNER JOIN dbo.flow_chain_version AS version
	ON version.chain_no = latest.chain_no
	AND version.version_id = latest.version_id
WHERE chain.[status] = 1
	AND chain.chain_no = @{chain_no}
ORDER BY chain.chain_no`

const LoadChainNodes = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	INNER JOIN dbo.flow_chain_definition AS chain
		ON chain.chain_no = version.chain_no
	WHERE chain.[status] = 1
		AND version.[status] = 1
	GROUP BY version.chain_no
)
SELECT
	node.node_id,
	node.version_id,
	node.chain_no,
	node.node_def_no,
	definition.[name] AS [name],
	definition.[type] AS [type],
	node.extparams,
	node.layout,
	definition.extparams AS definition_extparams,
	definition.layout_definition,
	definition.[desc] AS [desc]
FROM dbo.flow_chain_node AS node
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = node.chain_no
	AND latest.version_id = node.version_id
INNER JOIN dbo.flow_node_definition AS definition
	ON definition.node_def_no = node.node_def_no
ORDER BY node.chain_no, node.version_id, node.node_id`

const LoadChainNodesByChainNo = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	INNER JOIN dbo.flow_chain_definition AS chain
		ON chain.chain_no = version.chain_no
	WHERE chain.[status] = 1
		AND version.[status] = 1
		AND version.chain_no = @{chain_no}
	GROUP BY version.chain_no
)
SELECT
	node.node_id,
	node.version_id,
	node.chain_no,
	node.node_def_no,
	definition.[name] AS [name],
	definition.[type] AS [type],
	node.extparams,
	node.layout,
	definition.extparams AS definition_extparams,
	definition.layout_definition,
	definition.[desc] AS [desc]
FROM dbo.flow_chain_node AS node
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = node.chain_no
	AND latest.version_id = node.version_id
INNER JOIN dbo.flow_node_definition AS definition
	ON definition.node_def_no = node.node_def_no
ORDER BY node.chain_no, node.version_id, node.node_id`

const LoadChainConnections = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	INNER JOIN dbo.flow_chain_definition AS chain
		ON chain.chain_no = version.chain_no
	WHERE chain.[status] = 1
		AND version.[status] = 1
	GROUP BY version.chain_no
)
SELECT
	connection.conn_id,
	connection.version_id,
	connection.chain_no,
	connection.from_id,
	connection.to_id,
	connection.[type] AS [type],
	connection.remark,
	connection.extparams
FROM dbo.flow_chain_connection AS connection
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = connection.chain_no
	AND latest.version_id = connection.version_id
ORDER BY connection.chain_no, connection.version_id, connection.conn_id`

const LoadChainConnectionsByChainNo = `--sql
WITH latest_active_version AS (
	SELECT
		version.chain_no,
		MAX(version.version_id) AS version_id
	FROM dbo.flow_chain_version AS version
	INNER JOIN dbo.flow_chain_definition AS chain
		ON chain.chain_no = version.chain_no
	WHERE chain.[status] = 1
		AND version.[status] = 1
		AND version.chain_no = @{chain_no}
	GROUP BY version.chain_no
)
SELECT
	connection.conn_id,
	connection.version_id,
	connection.chain_no,
	connection.from_id,
	connection.to_id,
	connection.[type] AS [type],
	connection.remark,
	connection.extparams
FROM dbo.flow_chain_connection AS connection
INNER JOIN latest_active_version AS latest
	ON latest.chain_no = connection.chain_no
	AND latest.version_id = connection.version_id
ORDER BY connection.chain_no, connection.version_id, connection.conn_id`
