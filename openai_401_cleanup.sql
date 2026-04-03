-- =============================================================================
-- Sub2API: 清理 OpenAI 401 账号
-- 数据库连接信息:
--   Host: 154.64.231.176  Port: 5490
--   User: sub2api  Database: sub2api
-- =============================================================================

-- 1. 查看 OpenAI 账号总数
SELECT count(*) AS openai_total FROM accounts WHERE platform = 'openai';

-- 2. 查看 401 错误账号数量
SELECT count(*) AS openai_401_total
FROM accounts
WHERE platform = 'openai' AND temp_unschedulable_reason LIKE '%401%';

-- 3. 查看 401 账号详情
SELECT id, name, platform, status, temp_unschedulable_reason
FROM accounts
WHERE platform = 'openai' AND temp_unschedulable_reason LIKE '%401%';

-- 4. 删除 401 账号（先删关联表，再删主表）
-- ⚠️ 确认无误后再执行以下语句

BEGIN;

-- 4a. 删除关联表数据
DELETE FROM account_groups WHERE account_id IN (
  SELECT id FROM accounts
  WHERE platform = 'openai' AND temp_unschedulable_reason LIKE '%401%'
);

-- 4b. 删除主表数据
DELETE FROM accounts
WHERE platform = 'openai' AND temp_unschedulable_reason LIKE '%401%';

COMMIT;

-- 5. 验证删除结果
SELECT count(*) AS remaining_openai FROM accounts WHERE platform = 'openai';
