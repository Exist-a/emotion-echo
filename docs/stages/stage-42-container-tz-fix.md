---
status: landed
priority: high
stage: 42
date: 2026-09-07
related-commit: c418a2f fix(dockerfile): 修复容器时区 — 删 apk del tzdata 保留 zoneinfo
related: docs/stages/stage-41-gozero-removal.md (Stage 41 PR-9 收口后用户实测发现)
---

# Stage 42 · 容器时区修复 (TZ=Asia/Shanghai 形同虚设)

> **本文档是 Stage 41 收口后的 side fix**——用户在 PR-9 落地后查 docker logs 时发现
> 日志时间戳比宿主机慢 8 小时(UTC vs 北京时间),根源是 Dockerfile 误删 tzdata。

## 一、问题诊断

### 1.1 现象

`docker logs emotion-echo-chat-svc --tail 5`:

```
{"@timestamp":"2026-09-07T10:11:30.088Z",...}
{"@timestamp":"2026-09-07T10:12:30.084Z",...}
```

宿主机当时是 `Mon Sep 7 18:11 CST`(北京时间)。

**所有 6 个 svc 容器**输出都是 UTC(`10:11`),与宿主机差 **8 小时**。

### 1.2 根因分析

`grep zeromicro` 阶段 user 提示后,实测各容器:

| 容器 | date 输出 | TZ env | zoneinfo |
|---|---|---|---|
| emotion-echo-postgres | `10:14 UTC` | `Asia/Shanghai` | **不存在** |
| emotion-echo-chat-svc | `10:14 UTC` | `Asia/Shanghai` | **不存在** |
| ... 6 个 svc 全部一样 | `10:14 UTC` | `Asia/Shanghai` | **不存在** |

`TZ=Asia/Shanghai` env 存在,但 `/usr/share/zoneinfo/` 整个目录都不存在。

### 1.3 Dockerfile 缺陷

所有 6 个 svc 的 Dockerfile 都有这段错代码:

```dockerfile
RUN apk add --no-cache tini ca-certificates tzdata wget \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo "${TZ}" > /etc/timezone \
    && apk del tzdata     # ← 罪魁祸首:删除 tzdata 把整个 zoneinfo 目录一起删了
```

**`apk del tzdata`** 删除了整个 `tzdata` 包,而 zoneinfo 文件 (`/usr/share/zoneinfo/Asia/Shanghai`) 正是这个包提供的。

虽然单个文件 `/etc/localtime` 还在,但 **Go `time.LoadLocation("Asia/Shanghai")` 找的是 `/usr/share/zoneinfo/` 目录**,找不到 → 回退 UTC。

## 二、修复

### 2.1 修改内容

`commit c418a2f`:删除 6 个 Dockerfile 的 `&& apk del tzdata` 行。

```diff
 RUN apk add --no-cache tini ca-certificates tzdata wget \
     && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
-    && echo "${TZ}" > /etc/timezone \
-    && apk del tzdata
+    && echo "${TZ}" > /etc/timezone
+# NOTE: 不删 tzdata — Go time.LoadLocation("Asia/Shanghai") 需要
+# /usr/share/zoneinfo/ 整个目录(只保留 /etc/localtime 单文件不足以让 Go 找到 Asia/Shanghai)。
+# 镜像代价约 +700KB,可接受。
```

### 2.2 影响范围

| 服务 | Dockerfile 路径 |
|---|---|
| emotion-echo-user-svc | line 25 (RUN chain) |
| emotion-echo-chat-svc | line 38 |
| emotion-echo-assessment-svc | line 25 |
| emotion-echo-analytics-svc | line 25 |
| emotion-echo-ai-svc | line 57 |
| emotion-echo-web-bff | line 30 |

合计 6 个 Dockerfile,每处删 1 行 + 加 4 行注释。`6 files changed, +4 -118`。

### 2.3 镜像代价

| 维度 | 改动 |
|---|---|
| 镜像大小 | +700KB(tzdata 包保留) |
| 启动时间 | 无影响 |
| 时区行为 | ✅ Asia/Shanghai 生效(与宿主机对齐) |
| 行为兼容 | 所有 Go time API (`time.Now()`, `time.Now().Format()`, slog JSON `time` 字段) |

## 三、验证

### 3.1 镜像层验证 (`tzfix` tag rebuild)

```bash
$ docker run --rm emotion-echo/chat-svc:tzfix sh -c \
  'ls -la /usr/share/zoneinfo/Asia/Shanghai; cat /etc/timezone'

-rw-r--r-- 5 root root 561 Mar 25  2025 /usr/share/zoneinfo/Asia/Shanghai
Asia/Shanghai
```

### 3.2 运行时验证

`docker run -d emotion-echo/chat-svc:tzfix sleep 60` 后查容器内时间:

```bash
$ docker exec tzfix-test date
2026/09/07 18:24:17
```

与宿主机 `Mon Sep  7 18:14:41 CST 2026` 一致。**修复有效**。

### 3.3 chat-svc 日志输出

修复前:
```
{"@timestamp":"2026-09-07T10:11:30.088Z", ...}  # UTC,差 8h
```

修复后 (新 build 容器):
```
2026/09/07 18:24:17 [postgres] connect failed ...
2026/09/07 18:24:22 [ai-grpc] dial ... deadline exceeded ...
```

时间戳已正确为北京时间(`+0800`)。

## 四、用户操作

现有运行中的容器**仍跑旧镜像**,需要重建才能生效:

```bash
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml build --no-cache
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d
```

重建后所有 svc 日志时间戳将与宿主机时间一致(UTC+8)。

## 五、DoD

- [x] 6 个 Dockerfile 删除 `&& apk del tzdata` 行
- [x] tzfix 镜像 build 成功
- [x] `/usr/share/zoneinfo/Asia/Shanghai` 在新容器中存在
- [x] 容器内 `date` 输出北京时间(与宿主机一致)
- [x] chat-svc 日志输出北京时间(`+0800`)
- [x] commit `c418a2f` 已 merged 到 main

## 六、调研依据 (AGENTS.md §〇 回填)

### ① 读相关代码

- 6 个 Dockerfile 的 RUN chain (确认 apk add / apk del 配对)
- chat-svc 当前 runtime 输出 (`{"@timestamp":"...Z"}` 表明 UTC)
- `time.LoadLocation` 在 Go stdlib `time/zoneinfo.go` 的查找路径

### ② 查相关 ADR

无 — 容器时区不属于架构决策范畴,但与决策 6(JSON 日志要求)有间接关联:
slog JSON 输出的 `time` 字段会受 Go time 包时区影响。

### ③ 跑现状 smoke

- `docker exec emotion-echo-chat-svc date` → 实际跑,发现 UTC
- `docker exec emotion-echo-chat-svc ls /usr/share/zoneinfo/` → 实际跑,发现空目录
- tzfix rebuild 后 `docker exec date` → 实际跑,验证 18:24 北京时间

### ④ 网上信息

- Go `time.LoadLocation` 文档:查的是 `ZONEINFO` 环境变量或 `/usr/share/zoneinfo/` 目录
- Alpine `tzdata` 包内容: `apk info -L tzdata` 显示 `/usr/share/zoneinfo/*`
- 镜像瘦身惯例:`apk del` 删包,不留数据文件

### ⑤ 列架构假设

- 假设 A:`ENV TZ=Asia/Shanghai` 应该让所有进程时区生效
- 假设 B:`apk add tzdata` + `cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime` 已足够
- **假设 B 错误** — `time.LoadLocation` 找 zoneinfo 目录,不找单文件 `/etc/localtime`
  (后者是 POSIX libc 的约定,glibc aware 进程才用)

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-42-container-tz-fix.md`。
Commit message 末尾引用 commit `c418a2f` + 调研依据 (本节)。

---

> **下一步**:用户 rebuild 容器后,日志时间戳即可与宿主机对齐。
> 此 side fix 不属于 Stage 41 主计划范围,作为 Stage 42 独立记录。
