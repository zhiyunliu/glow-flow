-- 样例数据库初始化(SQL Server)，可重复执行
IF DB_ID(N'glowflow_example') IS NULL
	CREATE DATABASE glowflow_example;
GO

USE glowflow_example;
GO

-- order_create 流程 xdb 节点写入的订单
IF OBJECT_ID(N'dbo.example_order', N'U') IS NULL
	CREATE TABLE dbo.example_order (
		id           BIGINT IDENTITY(1, 1) NOT NULL PRIMARY KEY,
		order_no     NVARCHAR(64)   NOT NULL,
		user_id      NVARCHAR(64)   NOT NULL,
		amount       DECIMAL(18, 2) NOT NULL,
		pay_amount   DECIMAL(18, 2) NOT NULL,
		total_qty    INT            NOT NULL,
		risk_level   NVARCHAR(16)   NULL,
		member_level NVARCHAR(16)   NULL,
		[status]     NVARCHAR(16)   NOT NULL,
		create_time  DATETIME       NOT NULL DEFAULT GETDATE()
	);
GO

-- device_alarm 流程 xdb 节点写入的告警记录
IF OBJECT_ID(N'dbo.example_alarm_log', N'U') IS NULL
	CREATE TABLE dbo.example_alarm_log (
		id          BIGINT IDENTITY(1, 1) NOT NULL PRIMARY KEY,
		device_id   NVARCHAR(64)   NOT NULL,
		alarm_level NVARCHAR(16)   NOT NULL,
		alarm_count INT            NOT NULL,
		content     NVARCHAR(1000) NOT NULL,
		create_time DATETIME       NOT NULL DEFAULT GETDATE()
	);
GO
