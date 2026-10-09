#!/usr/bin/env bash
#
# scripts/check_docker_digests.sh — Round 4.7 PR-3 digest pin 校验
#
# 目的：扫描全仓 Dockerfile，断言 FROM 行必须用 image@sha256:... 形式
# （锁死基础镜像版本，避免 :latest 漂移 + docker hub tag 删除风险）。
#
# 退出码：
#   0 = 所有 Dockerfile 已 digest pinned
#   1 = 存在未 pinned 的 FROM（CI 红）
#
# 例外：注释行（行首 #）跳过；带 # 临时注释掉的 FROM 也跳过。
#
# 用法：
#   bash scripts/check_docker_digests.sh
# 或 CI：
#   - name: Check Dockerfile digest pinning
#     run: bash scripts/check_docker_digests.sh

set -uo pipefail

# 查找全仓 Dockerfile（排除 node_modules / .git / vendor / models / legacy）
# 排除：
#   - node_modules / .git / vendor: 第三方依赖目录
#   - emotion-echo-models/*: AI 模型预烘焙镜像（Stage 60 PR-TTS-VENDOR 已 vendor 化），
#     实际部署走 docker.io vendor 镜像，仓内 Dockerfile 仅作 reference
#   - legacy/*: 已退役工程，保留仅作历史快照
mapfile -t dockerfiles < <(find . \
    -name "Dockerfile" \
    -o -name "Dockerfile.*" \
    | grep -v "^./node_modules/" \
    | grep -v "^./.git/" \
    | grep -v "/vendor/" \
    | grep -v "^./emotion-llm-service/__pycache__/" \
    | grep -v "^./emotion-echo-models/" \
    | grep -v "^./legacy/" \
    | sort)

if [ "${#dockerfiles[@]}" -eq 0 ]; then
    echo "ERR: no Dockerfile found"
    exit 2
fi

total=0
unpinned=0
unpinned_files=()

for f in "${dockerfiles[@]}"; do
    while IFS= read -r line; do
        # 跳过空行 + 注释行
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

        # 只看 FROM 行（不区分大小写，但 Dockerfile 关键字都是大写）
        if [[ "$line" =~ ^[[:space:]]*FROM[[:space:]] ]]; then
            total=$((total + 1))
            # 必须含 @sha256:（字面） OR ${VAR:-...} digest env var 形式
            # （Round D：接受 ARG + env var 模式，因为 Dockerfile.digests.lock 真值
            #  通过 build args 注入；AI-svc 已用此模式，其他 6 Dockerfile 待迁移）
            if [[ ! "$line" =~ @sha256: ]] && [[ ! "$line" =~ \$\{[A-Z_0-9]+_DIGEST ]]; then
                unpinned=$((unpinned + 1))
                unpinned_files+=("$f: $line")
            fi
        fi
    done < "$f"
done

# 检查占位值（全零 digest）
placeholder=0
placeholder_files=()

# 从 Dockerfile.digests.lock 读取占位值
lock_file="Dockerfile.digests.lock"
if [ -f "$lock_file" ]; then
    while IFS= read -r line; do
        # 跳过空行和注释
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
        # 检查 sha256:000...000 占位值
        if [[ "$line" =~ sha256:0{10,} ]]; then
            placeholder=$((placeholder + 1))
            placeholder_files+=("$line")
        fi
    done < "$lock_file"
fi

# 也检查 Dockerfile 中的占位值
for f in "${dockerfiles[@]}"; do
    while IFS= read -r line; do
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
        if [[ "$line" =~ ^[[:space:]]*FROM[[:space:]] ]]; then
            if [[ "$line" =~ sha256:0{10,} ]]; then
                placeholder=$((placeholder + 1))
                placeholder_files+=("$f: $line")
            fi
        fi
    done < "$f"
done

# ===== compose `image:` 行（E2E-F-188，2026-10-09 扩扫）=====
#
# 背景（账本 E2E-F-188）：本守卫原先 `find -name Dockerfile*` **只扫 Dockerfile**，
# compose 的 `image:` 行既无 pin 机制也无守卫 ⇒ `quay.io/minio/minio:latest` 这类
# 浮动引用可以长期存在，而**没有任何门禁看得见**。
#
# 规则（与 Dockerfile 段同精神，但适配 compose 的现实）：
#   · **第三方镜像**：必须 `@sha256:` digest-pinned，或带**具体 tag**
#     （禁 `:latest`、禁无 tag）。`latest` 是版本漂移与"tag 被删"的高发点。
#   · **自建镜像**（路径含 `emotion-echo/`，含 ACR 上的）：**豁免 digest 要求**
#     —— 它们由本仓构建、digest 每次构建都变，钉 digest 会打断"本地构建→验收"闭环；
#     但**必须有非 latest 的版本 tag**（版本即契约）。
# 注册表不可达时（D-07 已知债）沿用既有语义：只报不改，退出码不受影响。
COMPOSE_GLOB="${COMPOSE_GLOB:-deploy/docker-compose*.yml deploy/compose*.yml}"

c_total=0
c_unpinned=0
c_unpinned_files=()

# shellcheck disable=SC2086  # 有意做 glob 展开
for f in $COMPOSE_GLOB; do
    [ -f "$f" ] || continue
    while IFS= read -r line; do
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
        [[ "$line" =~ ^[[:space:]]*image:[[:space:]]* ]] || continue
        ref="${line#*image:}"
        ref="${ref%%#*}"                                   # 去行尾注释
        ref="$(printf '%s' "$ref" | xargs || true)"         # trim
        [ -z "$ref" ] && continue
        # `scratch` 不是可拉取的引用（Docker 的空基础镜像），无 tag/digest 概念；
        # 本仓唯一用处是 compose.prod.yml 里 `profiles: ["never"]` 的占位服务
        # `_prod_overrides_pending`（永不启动）。
        [ "$ref" = "scratch" ] && continue
        c_total=$((c_total + 1))
        # tag = 最后一个路径段里 `:` 之后的部分（对 registry:port/name 也正确）
        last="${ref##*/}"
        if [[ "$last" == *:* ]]; then tag="${last##*:}"; else tag=""; fi

        if [[ "$ref" == *"emotion-echo/"* ]]; then
            if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
                c_unpinned=$((c_unpinned + 1))
                c_unpinned_files+=("$f: $ref  （自建镜像须带具体版本 tag，禁 :latest/无 tag）")
            fi
        else
            if [[ ! "$ref" =~ @sha256: ]] && { [ -z "$tag" ] || [ "$tag" = "latest" ]; }; then
                c_unpinned=$((c_unpinned + 1))
                c_unpinned_files+=("$f: $ref  （第三方须 @sha256: 或具体 tag，禁 :latest/无 tag）")
            fi
        fi
    done < "$f"
done

echo "Dockerfile digest pin check:"
echo "  total FROM: $total"
echo "  unpinned:   $unpinned"
echo "  placeholder: $placeholder"
echo "compose image pin check (E2E-F-188):"
echo "  total image: $c_total"
echo "  unpinned:    $c_unpinned"

if [ "$unpinned" -gt 0 ] || [ "$c_unpinned" -gt 0 ]; then
    echo ""
    echo "FAIL: 以下引用未 pin（Dockerfile 须 image@sha256:...；compose 第三方须 @sha256: 或具体 tag）："
    [ "$unpinned" -gt 0 ] && printf '  %s\n' "${unpinned_files[@]}"
    [ "$c_unpinned" -gt 0 ] && printf '  %s\n' "${c_unpinned_files[@]}"
    exit 1
fi

# ---------- 负向对照：写回未 pin 的 tag ⇒ 必须 RED ----------
# E2E-30 #13 要求（"写回未 pin 的 tag ⇒ RED"）。用 COMPOSE_GLOB 把扫描面指向一个临时
# compose（其余检查不变），断言**退出码非零** —— 先看退出码再解释输出（本仓库教训：
# "没输出"不等于"结果正确"）。子进程用 COMPOSE_NEG_CHILD 标记防递归。
if [ -z "${COMPOSE_NEG_CHILD:-}" ]; then
    _neg_dir="$(mktemp -d)"
    printf 'services:\n  probe:\n    image: example/probe:latest\n' \
        > "$_neg_dir/docker-compose.neg.yml"
    if COMPOSE_GLOB="$_neg_dir/docker-compose*.yml" COMPOSE_NEG_CHILD=1 \
        bash "$0" >/dev/null 2>&1; then
        echo "FAIL: 负向对照未触发 —— 写回 ':latest' 的 compose 没有被判红（守卫是弱断言）"
        rm -rf "$_neg_dir"
        exit 1
    fi
    rm -rf "$_neg_dir"
    echo "负向对照 OK：写回 ':latest' 的 compose 引用 ⇒ RED"
fi

if [ "$placeholder" -gt 0 ]; then
    echo ""
    echo "⚠️  WARNING: 发现 $placeholder 个占位 digest（sha256:000...000）："
    printf '  %s\n' "${placeholder_files[@]}"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "KNOWN DEBT (D-07): 占位 digest 等同于未 pin"
    echo ""
    echo "原因：Docker registry 在当前环境不可达（curl HTTP 000 超时）"
    echo "      无法自动回填真实 digest 值"
    echo ""
    echo "复检条件：当网络可达时，运行以下命令回填真值："
    echo "  bash scripts/sync_docker_digests.sh"
    echo ""
    echo "风险：基础镜像版本未锁定，可能因 :latest 漂移或 tag 删除导致构建失败"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "WARN: 占位 digest 已记录为已知缺口（非静默通过）"
    exit 0
fi

echo ""
echo "OK: all $total FROM lines are digest pinned (no placeholders)"
