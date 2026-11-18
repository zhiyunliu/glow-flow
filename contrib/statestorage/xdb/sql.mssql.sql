IF DB_ID(N'glowflow') IS NULL
BEGIN
	EXEC(N'CREATE DATABASE glowflow COLLATE Chinese_PRC_CI_AS');
END;
GO

USE glowflow;
GO

IF OBJECT_ID(N'dbo.flow_instance_state', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_instance_state (
		instance_id varchar(64) NOT NULL,
		chain_id varchar(32) NOT NULL,
		[version] varchar(32) NOT NULL,
		[status] varchar(16) NOT NULL,
		state_data nvarchar(max) NOT NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_instance_state_create_time DEFAULT (GETDATE()),
		update_time datetime NOT NULL CONSTRAINT DF_flow_instance_state_update_time DEFAULT (GETDATE()),
		CONSTRAINT PK_flow_instance_state PRIMARY KEY CLUSTERED (instance_id)
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_node_state', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_node_state (
		instance_id varchar(64) NOT NULL,
		node_id varchar(64) NOT NULL,
		chain_id varchar(32) NOT NULL,
		[version] varchar(32) NOT NULL,
		[status] varchar(16) NOT NULL,
		attempts int NOT NULL CONSTRAINT DF_flow_node_state_attempts DEFAULT (0),
		state_data nvarchar(max) NOT NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_node_state_create_time DEFAULT (GETDATE()),
		update_time datetime NOT NULL CONSTRAINT DF_flow_node_state_update_time DEFAULT (GETDATE()),
		CONSTRAINT PK_flow_node_state PRIMARY KEY CLUSTERED (instance_id, node_id)
	);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_instance_state_chain_version_status' AND object_id = OBJECT_ID(N'dbo.flow_instance_state'))
BEGIN
	CREATE INDEX IX_flow_instance_state_chain_version_status ON dbo.flow_instance_state (chain_id, [version], [status]);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_instance_state_status' AND object_id = OBJECT_ID(N'dbo.flow_instance_state'))
BEGIN
	CREATE INDEX IX_flow_instance_state_status ON dbo.flow_instance_state ([status]);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_node_state_chain_status' AND object_id = OBJECT_ID(N'dbo.flow_node_state'))
BEGIN
	CREATE INDEX IX_flow_node_state_chain_status ON dbo.flow_node_state (chain_id, [status]);
END;
GO

DECLARE @descriptions TABLE (
	schema_name sysname NOT NULL,
	table_name sysname NOT NULL,
	column_name sysname NULL,
	description nvarchar(4000) NOT NULL
);

INSERT INTO @descriptions (schema_name, table_name, column_name, description)
VALUES
	(N'dbo', N'flow_instance_state', NULL, N'流程实例状态'),
	(N'dbo', N'flow_instance_state', N'instance_id', N'流程实例 ID'),
	(N'dbo', N'flow_instance_state', N'chain_id', N'流程链编号'),
	(N'dbo', N'flow_instance_state', N'version', N'流程链版本'),
	(N'dbo', N'flow_instance_state', N'status', N'实例状态：pending/running/paused/completed/failed/canceled'),
	(N'dbo', N'flow_instance_state', N'state_data', N'完整实例状态，对应 InstanceState，JSON 格式'),
	(N'dbo', N'flow_instance_state', N'create_time', N'创建时间'),
	(N'dbo', N'flow_instance_state', N'update_time', N'更新时间'),

	(N'dbo', N'flow_node_state', NULL, N'流程节点状态'),
	(N'dbo', N'flow_node_state', N'instance_id', N'流程实例 ID'),
	(N'dbo', N'flow_node_state', N'node_id', N'节点 ID'),
	(N'dbo', N'flow_node_state', N'chain_id', N'流程链编号'),
	(N'dbo', N'flow_node_state', N'version', N'流程链版本'),
	(N'dbo', N'flow_node_state', N'status', N'节点状态：pending/running/completed/failed/skipped'),
	(N'dbo', N'flow_node_state', N'attempts', N'执行次数'),
	(N'dbo', N'flow_node_state', N'state_data', N'完整节点状态，对应 NodeState，JSON 格式'),
	(N'dbo', N'flow_node_state', N'create_time', N'创建时间'),
	(N'dbo', N'flow_node_state', N'update_time', N'更新时间');

DECLARE @schema_name sysname;
DECLARE @table_name sysname;
DECLARE @column_name sysname;
DECLARE @description nvarchar(4000);
DECLARE @major_id int;
DECLARE @minor_id int;

DECLARE description_cursor CURSOR LOCAL FAST_FORWARD FOR
SELECT schema_name, table_name, column_name, description
FROM @descriptions;

OPEN description_cursor;
FETCH NEXT FROM description_cursor INTO @schema_name, @table_name, @column_name, @description;

WHILE @@FETCH_STATUS = 0
BEGIN
	SELECT @major_id = t.object_id
	FROM sys.tables AS t
	INNER JOIN sys.schemas AS s ON s.schema_id = t.schema_id
	WHERE s.[name] = @schema_name AND t.[name] = @table_name;

	IF @major_id IS NOT NULL
	BEGIN
		IF @column_name IS NULL
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM sys.extended_properties
				WHERE class = 1 AND major_id = @major_id AND minor_id = 0 AND [name] = N'MS_Description'
			)
			BEGIN
				EXEC sys.sp_updateextendedproperty
					@name = N'MS_Description',
					@value = @description,
					@level0type = N'SCHEMA', @level0name = @schema_name,
					@level1type = N'TABLE', @level1name = @table_name;
			END
			ELSE
			BEGIN
				EXEC sys.sp_addextendedproperty
					@name = N'MS_Description',
					@value = @description,
					@level0type = N'SCHEMA', @level0name = @schema_name,
					@level1type = N'TABLE', @level1name = @table_name;
			END;
		END
		ELSE
		BEGIN
			SELECT @minor_id = c.column_id
			FROM sys.columns AS c
			WHERE c.object_id = @major_id AND c.[name] = @column_name;

			IF @minor_id IS NOT NULL
			BEGIN
				IF EXISTS (
					SELECT 1
					FROM sys.extended_properties
					WHERE class = 1 AND major_id = @major_id AND minor_id = @minor_id AND [name] = N'MS_Description'
				)
				BEGIN
					EXEC sys.sp_updateextendedproperty
						@name = N'MS_Description',
						@value = @description,
						@level0type = N'SCHEMA', @level0name = @schema_name,
						@level1type = N'TABLE', @level1name = @table_name,
						@level2type = N'COLUMN', @level2name = @column_name;
				END
				ELSE
				BEGIN
					EXEC sys.sp_addextendedproperty
						@name = N'MS_Description',
						@value = @description,
						@level0type = N'SCHEMA', @level0name = @schema_name,
						@level1type = N'TABLE', @level1name = @table_name,
						@level2type = N'COLUMN', @level2name = @column_name;
				END;
			END;
		END;
	END;

	SET @major_id = NULL;
	SET @minor_id = NULL;
	FETCH NEXT FROM description_cursor INTO @schema_name, @table_name, @column_name, @description;
END;

CLOSE description_cursor;
DEALLOCATE description_cursor;
GO
