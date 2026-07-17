-- JuanNiang-Neo 数据库初始化脚本 (Postgres)

-- 创建数据库 (需手动执行)
-- CREATE DATABASE juan;

-- 注意: 表结构由 GORM AutoMigrate 自动创建, 此文件仅保留参考。
-- 启动服务后 GORM 会自动建表, 无需手动执行本脚本。

-- 首次启动后自动创建的管理员账户:
-- username: admin
-- password: Admin123
-- (登录 Web 管理面板后建议立即修改)

-- ==========================================
-- 手动参考表结构 (由 GORM 自动管理)
-- ==========================================

-- admin_users: 管理员用户表
-- providers: LLM Provider 配置表
-- mcp_servers: MCP 服务器配置表
-- skills: 技能配置表
-- tool_configs: 工具配置表
-- prompts: 提示词模板表
-- chat_areas: 聊天区域表
-- sessions: 会话表
-- short_term_memories: 短期记忆配置表
-- long_term_memories: 长期记忆配置表
-- long_term_memory_items: 长期记忆条目表
-- background_tasks: 后台任务表
-- chat_records: 聊天记录表
-- plugins: Lua 插件元数据表
-- acl_rules: ACL 规则表
