"""从 roadmap.md 里解析出所有状态为 partial 的阶段目录名。

被 scripts/check_stage_todo_section.sh 调用。单独成文件而不是塞进 heredoc，
是因为 `$(...)` 里嵌 python heredoc 在 Git Bash 下会吞掉 stdin
（E2E-23 实测：静默退出码 49、零输出，守卫于是把"没检查到"当成"检查通过"）。

**工具自身失败必须报红**：找不到 roadmap / 解析异常时以非零退出，
调用方据此判红，不允许静默返回空列表。
**同样地，找不到唯一目录的阶段必须报出来**，而不是悄悄跳过 ——
悄悄跳过等于那个阶段从此不受守卫管，正是这个守卫要防的事。
"""
import io
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ROADMAP = os.path.join(REPO, "docs", "e2e-roadmap", "roadmap.md")
STAGES = os.path.join(REPO, "docs", "e2e-roadmap", "stages")

CELL = re.compile(r"\(stages/([A-Za-z0-9_.-]+)/")


def main() -> int:
    if not os.path.isfile(ROADMAP):
        sys.stderr.write("FATAL: 找不到 %s\n" % ROADMAP)
        return 2
    try:
        text = io.open(ROADMAP, encoding="utf-8").read()
        entries = sorted(os.listdir(STAGES))
    except OSError as e:
        sys.stderr.write("FATAL: 读取失败：%s\n" % e)
        return 2

    out = []
    for line in text.split("\n"):
        if not line.startswith("| E2E-"):
            continue
        cols = [c.strip() for c in line.strip("|").split("|")]
        if len(cols) < 5 or "partial" not in cols[4]:
            continue
        m = CELL.search(line)
        if m:
            out.append(m.group(1))
            continue
        num = cols[0].split()[0]                      # 如 "E2E-03"
        base = "e2e-" + num.split("-")[-1]            # 如 "e2e-03"
        cands = [d for d in entries if d.startswith(base)]
        if len(cands) == 1:
            out.append(cands[0])
        else:
            sys.stderr.write(
                "WARN: 阶段 %s 标 partial，但无法唯一确定目录（候选=%s）——该阶段未被守卫覆盖\n"
                % (num, cands)
            )

    # ⚠️ 用 sys.stdout.write 而不是 print：Windows 下 print 发 \r\n，
    # 而 shell 的 $(...) 只剥掉换行、会把 \r 留下，拼路径必失败（E2E-23 实测踩过）。
    for d in sorted(set(out)):
        sys.stdout.write(d.strip() + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
