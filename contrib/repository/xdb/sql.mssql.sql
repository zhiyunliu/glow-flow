IF DB_ID(N'glowflow') IS NULL
BEGIN
	EXEC(N'CREATE DATABASE glowflow COLLATE Chinese_PRC_CI_AS');
END;
GO

USE glowflow;
GO

IF OBJECT_ID(N'dbo.flow_chain_definition', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_chain_definition (
		chain_no varchar(32) NOT NULL,
		[name] varchar(256) NOT NULL,
		[status] int NOT NULL CONSTRAINT DF_flow_chain_definition_status DEFAULT (1),
		extparams varchar(max) NULL,
		layout varchar(max) NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_chain_definition_create_time DEFAULT (GETDATE()),
		create_by varchar(32) NULL,
		[operator] varchar(32) NULL,
		update_time datetime NULL,
		[desc] varchar(1024) NULL,
		CONSTRAINT PK_flow_chain_definition PRIMARY KEY CLUSTERED (chain_no),
		CONSTRAINT CK_flow_chain_definition_status CHECK ([status] IN (0, 1))
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_chain_version', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_chain_version (
		version_id bigint IDENTITY(10000, 1) NOT NULL,
		chain_no varchar(32) NOT NULL,
		[name] varchar(256) NOT NULL,
		[status] int NOT NULL CONSTRAINT DF_flow_chain_version_status DEFAULT (1),
		extparams varchar(max) NULL,
		layout varchar(max) NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_chain_version_create_time DEFAULT (GETDATE()),
		create_by varchar(32) NULL,
		[operator] varchar(32) NULL,
		update_time datetime NULL,
		[desc] varchar(1024) NULL,
		CONSTRAINT PK_flow_chain_version PRIMARY KEY CLUSTERED (version_id),
		CONSTRAINT CK_flow_chain_version_status CHECK ([status] IN (0, 1))
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_node_definition', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_node_definition (
		node_def_no varchar(32) NOT NULL,
		[name] varchar(128) NOT NULL,
		[type] varchar(64) NOT NULL,
		extparams varchar(max) NULL,
		layout_definition varchar(max) NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_node_definition_create_time DEFAULT (GETDATE()),
		create_by varchar(32) NULL,
		[operator] varchar(32) NULL,
		update_time datetime NULL,
		[desc] varchar(1024) NULL,
		CONSTRAINT PK_flow_node_definition PRIMARY KEY CLUSTERED (node_def_no)
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_layout_definition', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_layout_definition (
		layout_no varchar(32) NOT NULL,
		control_type varchar(32) NOT NULL,
		definition varchar(max) NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_layout_definition_create_time DEFAULT (GETDATE()),
		create_by varchar(32) NULL,
		[operator] varchar(32) NULL,
		update_time datetime NULL,
		[desc] varchar(1024) NULL,
		CONSTRAINT PK_flow_layout_definition PRIMARY KEY CLUSTERED (layout_no)
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_basic_infra', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_basic_infra (
		infra_no varchar(32) NOT NULL,
		infra_type varchar(32) NOT NULL,
		extparams varchar(max) NULL,
		create_time datetime NOT NULL CONSTRAINT DF_flow_basic_infra_create_time DEFAULT (GETDATE()),
		create_by varchar(32) NULL,
		[operator] varchar(32) NULL,
		update_time datetime NULL,
		[desc] varchar(1024) NULL,
		CONSTRAINT PK_flow_basic_infra PRIMARY KEY CLUSTERED (infra_no)
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_chain_node', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_chain_node (
		node_id bigint IDENTITY(1000, 1) NOT NULL,
		version_id bigint NOT NULL,
		chain_no varchar(32) NOT NULL,
		node_def_no varchar(32) NOT NULL,
		extparams varchar(max) NULL,
		layout varchar(max) NULL,
		CONSTRAINT PK_flow_chain_node PRIMARY KEY CLUSTERED (node_id)
	);
END;
GO

IF OBJECT_ID(N'dbo.flow_chain_connection', N'U') IS NULL
BEGIN
	CREATE TABLE dbo.flow_chain_connection (
		conn_id bigint IDENTITY(1000, 1) NOT NULL,
		version_id bigint NOT NULL,
		chain_no varchar(32) NOT NULL,
		from_id varchar(32) NOT NULL,
		to_id varchar(32) NOT NULL,
		[type] varchar(64) NOT NULL,
		remark varchar(256) NULL,
		extparams varchar(max) NULL,
		CONSTRAINT PK_flow_chain_connection PRIMARY KEY CLUSTERED (conn_id)
	);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_chain_version_chain_no' AND object_id = OBJECT_ID(N'dbo.flow_chain_version'))
BEGIN
	CREATE INDEX IX_flow_chain_version_chain_no ON dbo.flow_chain_version (chain_no);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_chain_node_version_chain' AND object_id = OBJECT_ID(N'dbo.flow_chain_node'))
BEGIN
	CREATE INDEX IX_flow_chain_node_version_chain ON dbo.flow_chain_node (version_id, chain_no);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_chain_node_node_def_no' AND object_id = OBJECT_ID(N'dbo.flow_chain_node'))
BEGIN
	CREATE INDEX IX_flow_chain_node_node_def_no ON dbo.flow_chain_node (node_def_no);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_chain_connection_version_chain' AND object_id = OBJECT_ID(N'dbo.flow_chain_connection'))
BEGIN
	CREATE INDEX IX_flow_chain_connection_version_chain ON dbo.flow_chain_connection (version_id, chain_no);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_chain_connection_from_to' AND object_id = OBJECT_ID(N'dbo.flow_chain_connection'))
BEGIN
	CREATE INDEX IX_flow_chain_connection_from_to ON dbo.flow_chain_connection (version_id, from_id, to_id);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_node_definition_type' AND object_id = OBJECT_ID(N'dbo.flow_node_definition'))
BEGIN
	CREATE INDEX IX_flow_node_definition_type ON dbo.flow_node_definition ([type]);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_layout_definition_control_type' AND object_id = OBJECT_ID(N'dbo.flow_layout_definition'))
BEGIN
	CREATE INDEX IX_flow_layout_definition_control_type ON dbo.flow_layout_definition (control_type);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE [name] = N'IX_flow_basic_infra_type' AND object_id = OBJECT_ID(N'dbo.flow_basic_infra'))
BEGIN
	CREATE INDEX IX_flow_basic_infra_type ON dbo.flow_basic_infra (infra_type);
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
	(N'dbo', N'flow_chain_definition', NULL, N'流程链定义'),
	(N'dbo', N'flow_chain_definition', N'chain_no', N'流程链编号'),
	(N'dbo', N'flow_chain_definition', N'name', N'名称'),
	(N'dbo', N'flow_chain_definition', N'status', N'状态：1 启用，0 禁用'),
	(N'dbo', N'flow_chain_definition', N'extparams', N'扩展参数，JSON 格式'),
	(N'dbo', N'flow_chain_definition', N'layout', N'布局配置，对应 Layout，JSON 格式'),
	(N'dbo', N'flow_chain_definition', N'create_time', N'创建时间'),
	(N'dbo', N'flow_chain_definition', N'create_by', N'创建人'),
	(N'dbo', N'flow_chain_definition', N'operator', N'操作人'),
	(N'dbo', N'flow_chain_definition', N'update_time', N'更新时间'),
	(N'dbo', N'flow_chain_definition', N'desc', N'描述'),

	(N'dbo', N'flow_chain_version', NULL, N'流程链版本'),
	(N'dbo', N'flow_chain_version', N'version_id', N'版本 ID，自增长'),
	(N'dbo', N'flow_chain_version', N'chain_no', N'流程链编号'),
	(N'dbo', N'flow_chain_version', N'name', N'名称'),
	(N'dbo', N'flow_chain_version', N'status', N'状态：1 启用，0 禁用'),
	(N'dbo', N'flow_chain_version', N'extparams', N'扩展参数，JSON 格式'),
	(N'dbo', N'flow_chain_version', N'layout', N'布局配置，对应 Layout，JSON 格式'),
	(N'dbo', N'flow_chain_version', N'create_time', N'创建时间'),
	(N'dbo', N'flow_chain_version', N'create_by', N'创建人'),
	(N'dbo', N'flow_chain_version', N'operator', N'操作人'),
	(N'dbo', N'flow_chain_version', N'update_time', N'更新时间'),
	(N'dbo', N'flow_chain_version', N'desc', N'描述'),

	(N'dbo', N'flow_node_definition', NULL, N'节点定义'),
	(N'dbo', N'flow_node_definition', N'node_def_no', N'节点定义编号'),
	(N'dbo', N'flow_node_definition', N'name', N'名称'),
	(N'dbo', N'flow_node_definition', N'type', N'节点类型'),
	(N'dbo', N'flow_node_definition', N'extparams', N'扩展参数，JSON 格式'),
	(N'dbo', N'flow_node_definition', N'layout_definition', N'布局定义'),
	(N'dbo', N'flow_node_definition', N'create_time', N'创建时间'),
	(N'dbo', N'flow_node_definition', N'create_by', N'创建人'),
	(N'dbo', N'flow_node_definition', N'operator', N'操作人'),
	(N'dbo', N'flow_node_definition', N'update_time', N'更新时间'),
	(N'dbo', N'flow_node_definition', N'desc', N'描述'),

	(N'dbo', N'flow_layout_definition', NULL, N'布局定义'),
	(N'dbo', N'flow_layout_definition', N'layout_no', N'布局编号'),
	(N'dbo', N'flow_layout_definition', N'control_type', N'控件类型'),
	(N'dbo', N'flow_layout_definition', N'definition', N'定义内容'),
	(N'dbo', N'flow_layout_definition', N'create_time', N'创建时间'),
	(N'dbo', N'flow_layout_definition', N'create_by', N'创建人'),
	(N'dbo', N'flow_layout_definition', N'operator', N'操作人'),
	(N'dbo', N'flow_layout_definition', N'update_time', N'更新时间'),
	(N'dbo', N'flow_layout_definition', N'desc', N'描述'),

	(N'dbo', N'flow_basic_infra', NULL, N'基础设施定义'),
	(N'dbo', N'flow_basic_infra', N'infra_no', N'基础设施编号'),
	(N'dbo', N'flow_basic_infra', N'infra_type', N'基础设施类型'),
	(N'dbo', N'flow_basic_infra', N'extparams', N'扩展参数，JSON 格式'),
	(N'dbo', N'flow_basic_infra', N'create_time', N'创建时间'),
	(N'dbo', N'flow_basic_infra', N'create_by', N'创建人'),
	(N'dbo', N'flow_basic_infra', N'operator', N'操作人'),
	(N'dbo', N'flow_basic_infra', N'update_time', N'更新时间'),
	(N'dbo', N'flow_basic_infra', N'desc', N'描述'),

	(N'dbo', N'flow_chain_node', NULL, N'流程链节点'),
	(N'dbo', N'flow_chain_node', N'node_id', N'节点 ID，自增长'),
	(N'dbo', N'flow_chain_node', N'version_id', N'版本 ID'),
	(N'dbo', N'flow_chain_node', N'chain_no', N'流程链编号'),
	(N'dbo', N'flow_chain_node', N'node_def_no', N'节点定义编号'),
	(N'dbo', N'flow_chain_node', N'extparams', N'扩展参数，JSON 格式'),
	(N'dbo', N'flow_chain_node', N'layout', N'布局配置，对应 Layout，JSON 格式'),

	(N'dbo', N'flow_chain_connection', NULL, N'流程链连接'),
	(N'dbo', N'flow_chain_connection', N'conn_id', N'连接 ID，自增长'),
	(N'dbo', N'flow_chain_connection', N'version_id', N'版本 ID'),
	(N'dbo', N'flow_chain_connection', N'chain_no', N'流程链编号'),
	(N'dbo', N'flow_chain_connection', N'from_id', N'来源节点定义编号，对应 node_def_no'),
	(N'dbo', N'flow_chain_connection', N'to_id', N'目标节点定义编号，对应 node_def_no'),
	(N'dbo', N'flow_chain_connection', N'type', N'连接类型'),
	(N'dbo', N'flow_chain_connection', N'remark', N'备注'),
	(N'dbo', N'flow_chain_connection', N'extparams', N'扩展参数，JSON 格式');

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
