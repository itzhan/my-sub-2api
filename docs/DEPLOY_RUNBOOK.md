# sub2api 部署 Runbook

> 用途：一份给 AI / 运维快速执行 my-sub-2api 升级部署的标准流程。
> 适用场景：本地仓库已 commit + push 到 `origin/main` 后，将新版本灰度滚动上线到生产。

---

## 0. 拓扑速查

| 角色 | 主机 | 容器 | 对外端口 | 镜像 tag |
|---|---|---|---|---|
| Master（控制面 / 调度 / 管理 UI） | `new1` (`154.64.231.176`) | `sub2api-master` | `0.0.0.0:8082→8080` | `sub2api-v2-master-sub2api:latest` |
| PostgreSQL（共享） | `new1` | `sub2api-postgres` | `5478→5432` | `postgres:18-alpine` |
| Redis（共享） | `new1` | `sub2api-redis` | `6378→6379` | `redis:8-alpine` |
| Workers ×12（数据面 / nginx 后端） | `server2` ~ `server13` | `sub2api-v2-worker` | `0.0.0.0:8082→8080` | `deploy-sub2api:latest` |
| 客户流量入口 | `new1` 上的 nginx | — | `:443` | — |

**关键 SSH 配置**（已在 `~/.ssh/config`）：`new1`、`server2` ~ `server13`。

**Compose project name**：`sub2api-v2-master`（必须显式 `-p`，否则会被推导成目录名 `deploy` 然后误起新 stack 端口冲突）。

**Compose 文件**：
- Master：`/opt/sub2api-v2/deploy/docker-compose.master.yml`
- Worker：`/opt/sub2api-v2/docker-compose.yml`（注意 worker 上的路径不同）

**镜像分发链**：
- new1 用 master compose 构建 → 默认产出 tag `deploy-sub2api:latest`
- 重 tag 一份为 `sub2api-v2-master-sub2api:latest` 给 master compose 用
- `docker save deploy-sub2api:latest | gzip` → 通过本地 Mac 中转分发给 workers（new1 不能直连 worker hostname）
- workers 加载后保留 tag `deploy-sub2api:latest`

---

## 1. 前置检查

在跑部署前确认：

```bash
# 1) 本地代码已 push
cd /Users/itzhan/Desktop/我的项目/codexProxy/proxy-server/sub2api魔改/my-sub-2api
git log --oneline origin/main..HEAD   # 应为空
git status                            # 应该 clean（除 frontend/package-lock.json untracked）

# 2) 本地编译/测试通过
cd backend
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go vet -tags=unit ./...
CGO_ENABLED=0 go vet -tags=integration ./...
CGO_ENABLED=0 go test -tags=unit -count=1 -timeout 10m ./... 2>&1 | grep -E "^(FAIL|---)" | head
# 唯一允许的失败是 TestAdminService_BulkUpdateAccounts_PartialFailureIDs（fork bulk 语义历史不兼容）

# 3) 前端 build 通过
cd ../frontend
pnpm install --frozen-lockfile
pnpm build

# 4) 看下要部署的 commit
git log --oneline -1
```

**遇到新 SQL migration 时额外检查**：

```bash
git diff --name-only @{1}..HEAD -- backend/migrations/
```

如有破坏性 migration（重命名列、删列、加非空唯一约束等），考虑先 `pg_dump` 备份。

---

## 2. 标准部署流程（5 步，每步独立可恢复）

### Step 1 — 同步代码到 new1

```bash
ssh new1 'cd /opt/sub2api-v2 && git fetch origin && git reset --hard origin/main && git log --oneline -1'
```

> 用 `reset --hard` 而非 `pull` 是因为 new1 上不会做本地修改；reset 等同于 fast-forward，幂等。

**验证**：输出的 commit hash 与本地 `git log --oneline -1` 一致。

---

### Step 2 — new1 构建镜像 + 重 tag

```bash
ssh new1 'cd /opt/sub2api-v2/deploy && docker compose -f docker-compose.master.yml build sub2api'
ssh new1 'docker tag deploy-sub2api:latest sub2api-v2-master-sub2api:latest'
ssh new1 'docker images deploy-sub2api:latest --format "{{.ID}}"'   # 记下新 image id
```

**关键**：`docker compose build` 默认产出 tag `deploy-sub2api:latest`（compose project=`deploy` 推导）；master compose 期待 tag 是 `sub2api-v2-master-sub2api:latest`。**忘记重 tag 会让 master 容器 recreate 后还跑旧镜像**（因为 `sub2api-v2-master-sub2api:latest` 仍指向旧 image id）。

**验证**：`docker images` 看到两个 tag 指向同一个 image id。

---

### Step 3 — 分发新镜像到 12 workers（不重启容器）

```bash
# 3a. 在 new1 上 save & gzip
ssh new1 'docker save deploy-sub2api:latest | gzip > /tmp/deploy-sub2api.tgz && ls -lh /tmp/deploy-sub2api.tgz'

# 3b. 拉到本地（new1 不能直连 worker hostname，必须中转）
scp -q new1:/tmp/deploy-sub2api.tgz /tmp/deploy-sub2api.tgz

# 3c. 并行分发 + load 到 server2..server13
for i in 2 3 4 5 6 7 8 9 10 11 12 13; do
  (
    scp -q -o ConnectTimeout=10 /tmp/deploy-sub2api.tgz server$i:/tmp/deploy-sub2api.tgz && \
    ssh server$i 'gunzip -c /tmp/deploy-sub2api.tgz | docker load 2>&1 | tail -1 && rm -f /tmp/deploy-sub2api.tgz' && \
    echo "=== server$i DONE ==="
  ) &
done
wait
echo "=== ALL DISTRIBUTED ==="
```

**预期**：每台输出 `Loaded image: deploy-sub2api:latest` + `=== serverN DONE ===`。

**验证**（可选）：抽查任一 worker 镜像 id 与 new1 一致：

```bash
ssh server7 'docker images deploy-sub2api:latest --format "{{.ID}}"'
```

> 此时 13 台节点都已有新镜像，但**所有容器仍跑旧镜像**，零业务影响。

---

### Step 4 — 升级 master（先动控制面）

```bash
ssh new1 'cd /opt/sub2api-v2/deploy && \
  docker compose -p sub2api-v2-master -f docker-compose.master.yml up -d --force-recreate sub2api'
```

**关键参数**：
- `-p sub2api-v2-master`：**必须显式指定**（避免 compose 当作新 project 起一份新 stack 端口冲突）
- `--force-recreate sub2api`：只重建 `sub2api` 服务，不动 postgres/redis

**等 healthy + 验证**：

```bash
ssh new1 'for i in 1 2 3 4 5 6 7 8 9 10; do
  st=$(docker inspect sub2api-master --format "{{.State.Health.Status}}" 2>/dev/null)
  h=$(curl -sfm 3 http://localhost:8082/health 2>/dev/null | head -c 30)
  echo "$i: health=$st curl=$h"
  [ "$st" = "healthy" ] && [ -n "$h" ] && break
  sleep 2
done
docker inspect sub2api-master --format "{{.Image}}" | cut -c8-24'
```

**验证 migration 已自动跑完**（每次 master 启动用 PostgreSQL Advisory Lock 串行执行）：

```bash
ssh new1 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "SELECT count(*), max(filename) FROM schema_migrations"'
```

> 客户流量走 nginx → workers，**升级 master 期间客户无感知**。Master 用于：调度器 leader 选举、outbox watermark 推进、admin UI、监控统计写入。短时不可用对客户请求无影响。

---

### Step 5 — 滚动升级 12 workers

```bash
for host in server2 server3 server4 server5 server6 server7 server8 server9 server10 server11 server12 server13; do
  echo "=========== $host ==========="
  ssh -o ConnectTimeout=8 $host "cd /opt/sub2api-v2 && docker compose up -d --force-recreate sub2api 2>&1 | tail -2"
  ssh -o ConnectTimeout=8 $host '
for i in 1 2 3 4 5 6 7 8 9 10 15; do
  st=$(docker inspect sub2api-v2-worker --format "{{.State.Health.Status}}" 2>/dev/null)
  [ "$st" = "healthy" ] && { echo "  healthy @ ${i}x ($((i*2))s)"; break; }
  sleep 2
done
echo -n "  img: "; docker inspect sub2api-v2-worker --format "{{.Image}}" | cut -c8-24'
done
echo "=== ROLL DONE ==="
```

**特征**：
- worker compose 没有名字冲突问题（不需要 `-p`）
- 单台升级耗时约 6 秒（recreate + healthcheck）
- 任意时刻只有 1 台 worker 不可用，nginx 自动切到其它 11 台
- 全部 12 台总耗时约 1~2 分钟

**遇到 ssh 偶发"Connection closed by port 22"**（fail2ban/速率限制）：等 5 秒重跑该单台即可，前面已成功的不影响。

---

## 3. 全量核验

```bash
echo "=== master ==="
ssh new1 "docker inspect sub2api-master --format '{{.Image}}' | cut -c8-24; curl -sfm 3 http://localhost:8082/health"

echo "=== workers ==="
for i in 2 3 4 5 6 7 8 9 10 11 12 13; do
  (ssh -o ConnectTimeout=8 server$i "img=\$(docker inspect sub2api-v2-worker --format '{{.Image}}' | cut -c8-24); h=\$(curl -sfm 3 http://localhost:8082/health 2>&1 | head -c 20); echo \"server$i: img=\$img health=\$h\"") &
done; wait
```

**预期**：13 行输出，所有 image id 与 new1 上构建的新 image id 一致；所有 health 都是 `{"status":"ok"}`。

---

## 4. 回滚（紧急情况）

> 旧镜像在升级时**不会被自动删除**（仅丢失 tag）。回滚 = 重 tag 旧镜像 → recreate 容器。

### 4.1 找回旧 image id

```bash
ssh new1 'docker images --filter "dangling=false" | grep -iE "sub2api|deploy" | head -10'
# 或精确：
ssh new1 'docker image inspect $(docker ps -a --format "{{.Image}}" | head) --format "{{.Id}} {{.RepoTags}}"' 2>/dev/null
```

历史 image id 可通过 `git log --grep "merge: sync upstream"` 找到对应 commit，再交叉对照之前部署日志。

### 4.2 回滚 master

```bash
ssh new1 'docker tag <旧image_id> sub2api-v2-master-sub2api:latest && \
  cd /opt/sub2api-v2/deploy && \
  docker compose -p sub2api-v2-master -f docker-compose.master.yml up -d --force-recreate sub2api'
```

### 4.3 回滚 workers

每台 worker 上：

```bash
ssh server<N> 'docker tag <旧image_id> deploy-sub2api:latest && \
  cd /opt/sub2api-v2 && \
  docker compose up -d --force-recreate sub2api'
```

> **数据库迁移不可自动回滚**。如果新版本带来了破坏性 schema 变更，回滚代码后旧代码可能读取不了新 schema。这种情况要么：
> a) 在新代码下保留兼容（默认行为，本仓库目前没有破坏性迁移）
> b) 提前 `pg_dump` 备份，回滚需手动 restore（注意会丢失部署后产生的新数据）

---

## 5. 已知坑 / 经验

### 5.1 PG 连接池容量

历史曾踩过 `max_connections=100` 被打满（每个 worker 4-8 条 + master 19 条 ≈ 87/100），psql 登录都进不去但业务连接仍工作。已**调到 300**（在 `/opt/sub2api-v2/deploy/postgres_data/postgresql.conf`）。如果再增加 worker 数量需评估。

### 5.2 Build 出来的 tag 名

`docker compose build` 的产出 tag 由 compose project name 推导。**新 1 上跑 `docker compose -f docker-compose.master.yml build`**（不带 `-p`）时 project name 取自工作目录（`deploy`）→ 镜像 tag 为 `deploy-sub2api:latest`。这正好和 worker tag 一致，所以 workers 直接用即可；但 master compose 配置写死 `image: sub2api-v2-master-sub2api:latest`（隐式由它的 project name 推导），所以**必须重 tag** 才能让 master 拿到新镜像。

### 5.3 docker compose down 的杀伤力

历史曾误用 `docker compose down --remove-orphans` 把整个 stack 删了（master + postgres + redis 三个容器都没了）。所幸 PG 和 Redis 是 bind mount（`/opt/sub2api-v2/deploy/postgres_data` / `redis_data`），数据保留。**永远只用 `up -d --force-recreate <service>`**，不要 down。

### 5.4 Migration 并发安全

代码用 `pg_advisory_lock` 串行化迁移：每个实例启动都尝试拿锁，第一个拿到的真正跑迁移，其它实例 wait 完看到 schema 已最新就跳过。所以 13 个实例同时启动也不会并发执行 migration，不必手动协调顺序。

### 5.5 唯一允许的单测失败

`TestAdminService_BulkUpdateAccounts_PartialFailureIDs` 是 fork commit `1a37d55c` 把 `BulkUpdateAccounts` 改为一次性 `BulkBindGroups` 后，与 upstream 引入的"per-account 部分失败"测试不兼容。本仓库已有此遗留 fail，**不算回归**。其它任何单测失败都需排查。

### 5.6 Free 账号会被调度去做生图

OAuth 账号池里 `plan_type=free` 的账号不支持 `image_generation` tool（Codex 会返回 400 "Tool choice 'image_generation' not found"）。当前调度器**没有按 plan 过滤**。如果生图错误率高，可临时跑 SQL 把 free 账号 `schedulable=false`：

```sql
UPDATE accounts SET schedulable=false
WHERE platform='openai' AND type='oauth'
  AND credentials::jsonb->>'plan_type'='free';
```

### 5.7 nginx ↔ 后端 ↔ OpenAI 三段超时

| 段 | 当前值 | 文件 |
|---|---|---|
| nginx → workers | 600s | `/opt/sub2api-v2/sub2api-nginx.conf`（`proxy_read_timeout`） |
| workers → OpenAI | 600s | `Gateway.ResponseHeaderTimeout`（viper 默认 600）|

生图大批量（n=4-8）已知会逼近 600s。客户端 timeout 必须 ≥ 10 分钟。

---

## 6. 标准时间预估

| 阶段 | 耗时 |
|---|---|
| Step 1 git pull | <5 秒 |
| Step 2 build + retag | 60-180 秒（看 frontend/backend cache 命中） |
| Step 3 分发到 12 workers | 30-60 秒（并行） |
| Step 4 升级 master + 等 healthy | 10-15 秒 |
| Step 5 滚动 12 workers | 1-2 分钟 |
| **总计** | **3-5 分钟** |

---

## 7. AI 执行小结提示

如果让 AI 执行此流程，推荐 prompt：

> 按照 `docs/DEPLOY_RUNBOOK.md` 部署当前 main 分支到生产。每个 Step 跑完汇报一次结果再继续，最后做全量核验。如果遇到任何单台 worker 失败，重试该单台后继续；如果 master 启动 healthy 探测超过 30 秒未就绪，立即停下来报告。

AI 应当：
- 跑命令前先 `git status` / `git log --oneline -1` 确认本地状态
- 每跑完 Step N 简短汇报（1-2 句）：新 image id、healthy 时间、是否需要人工介入
- 不要并行跑 Step 4 和 Step 5（master 必须先于 workers，让 master 跑完 migration）
- 不要在 Step 3 之前重启任何容器（避免没有新镜像就先停容器）
- 全程不需要修改 PG / Redis 配置，不需要碰 nginx
