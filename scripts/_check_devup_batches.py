"""检查 scripts/dev-up.sh 的分批等待语义（E2E-23 测试点 #29）。

断言两件事：

1. **覆盖** —— 每批 `$COMPOSE up -d` 起来的容器，都在本批内被 `wait_healthy`
   或 `wait_nacos` 等到。原先批3 起 7 个服务却只等 web-bff。

2. **无跨批错配** —— 本批 `wait_healthy` 的对象必须属于本批。
   原 bug：批2 起的是 kafka/nacos/minio/apisix，却写着
   `wait_healthy postgres 30`（postgres 属批1 且已等过），
   等于本批真正需要等的对象没人管，而重复等了个无关对象。

独立成文件而非内嵌 heredoc：`$(...)` 里嵌 python heredoc 在 Git Bash 下
会吞掉 stdin（实测退出码 49、零输出），脚本却继续往下跑，把"没检查到"
当成"检查通过" —— 假绿比没有守卫更危险。

输出约定：CHECKED=<n> 每行；PROBLEM <描述> 每行（由调用方转成 FAIL）。
退出码恒为 0 —— 判定逻辑在 shell 侧，避免"解析失败"被误读成"检查通过"。
"""

import re
import sys

ONE_SHOT = {
    # 一次性任务：跑完即退出，wait_healthy 对它们无意义
    "emotion-echo-db-migrate",
    "emotion-echo-apisix-seed",
    "emotion-echo-kafka-init",
    "emotion-echo-minio-init",
    # 前端容器用 --spider 探针且启动慢，批3 显式不列入等待
    "emotion-echo-web",
    # XTTS 是 2.6G 模型大户，dev-up.sh 批4 明确注释「避开峰值最后起」，
    # 刻意不阻塞等它 —— 改了会让 dev-up 多花 40~180s 却没有收益。
    "emotion-echo-xtts",
    # etcd：被 apisix 的 `depends_on: {etcd: {condition: service_healthy}}`
    # 保护（infra.yml:317-318），compose 自己会等，不必脚本再等一遍。
    "etcd",
}

# 由专用探针函数（而非 wait_healthy）覆盖的服务。
# nacos 走 wait_nacos 走 HTTP actuator/health，不是 .State.Health.Status。
ALT_PROBE = {"nacos": "wait_nacos"}


def strip_comment(line):
    """去掉 shell 注释（尊重引号）。

    必须剥离：dev-up.sh 的注释里含历史代码片段，比如
    「原先写的是 `wait_healthy postgres 30`」—— 不剥离就会把
    这段历史说明当成真实调用来断言，第一版就因此误报了 4 条 FAIL。
    """
    out = []
    quote = None
    for ch in line:
        if quote:
            out.append(ch)
            if ch == quote:
                quote = None
        elif ch in ("'", '"'):
            quote = ch
            out.append(ch)
        elif ch == "#":
            break
        else:
            out.append(ch)
    return "".join(out)


def tokenize(text):
    return [t for t in re.findall(r'"[^"]*"|\S+', text) if t != "\\"]


def parse_batches(lines):
    """把 dev-up.sh 切成若干批，返回 [{up:[...], wait:[...], loop:bool}]。"""
    batches = []
    cur = None
    i = 0
    while i < len(lines):
        code = strip_comment(lines[i]).strip()
        # 只认以 $COMPOSE up -d 开头的行。原先用 "up -d" in code 会把
        # 批3.5 那段 `curl ... | python -c "..."` 里恰好含 `up -d` 的文本
        # 也当成一批 up，解析出 "完成；等" / "healthy" / "===" 这种
        # 根本不是服务名的垃圾 token（第一版就误报了 3 条）。
        if re.match(r"^\$?\{?COMPOSE\}?\s+up\s+-d\b", code) or re.match(r"^COMPOSE\s+up\s+-d\b", code):
            if cur:
                batches.append(cur)
            cur = {"up": [], "wait": [], "alt": [], "loop": False}
            m = re.search(r"up -d (.*)$", code)
            if m:
                cur["up"] += tokenize(m.group(1))
            i += 1
            # 续行：up -d a b \  /  c d
            while i < len(lines):
                nxt = strip_comment(lines[i]).strip()
                if not nxt.endswith("\\"):
                    break
                cur["up"] += tokenize(nxt)
                i += 1
            continue
        if cur is not None:
            if re.search(r"wait_nacos", code):
                cur.setdefault("alt", []).append("nacos")
            if re.search(r'wait_healthy\s+"\$', code):
                cur["loop"] = True  # 循环里等一批，视为覆盖本批全部
            else:
                m = re.search(r"wait_healthy\s+(\S+)", code)
                if m:
                    cur["wait"].append(m.group(1))
        i += 1
    if cur:
        batches.append(cur)
    return batches


def main():
    if len(sys.argv) != 2:
        print("usage: _check_devup_batches.py <dev-up.sh>", file=sys.stderr)
        return 2

    with open(sys.argv[1], encoding="utf-8") as fh:
        lines = fh.read().split("\n")

    batches = parse_batches(lines)
    problems = []
    checked = 0

    for idx, b in enumerate(batches, 1):
        ups = [s for s in b["up"] if s]
        if not ups:
            continue
        waited = set(b["wait"])
        for svc in ups:
            if svc in ONE_SHOT:
                continue
            checked += 1
            alt = set(b.get("alt", []))
            covered = svc in waited or svc in alt
            if not b["loop"] and not covered:
                problems.append(
                    f"批{idx}: {svc} 在本批 up 起来但本批未 wait_healthy（也没走循环等待）"
                )
        if not b["loop"]:
            for w in b["wait"]:
                if w in ups:
                    continue
                problems.append(
                    f"批{idx}: wait_healthy {w} 但本批 up 的是 {sorted(ups)} —— 等了个无关对象"
                )

    print(f"CHECKED={checked}")
    print(f"BATCHES={len(batches)}")
    for p in problems:
        print("PROBLEM " + p)
    return 0


if __name__ == "__main__":
    sys.exit(main())
