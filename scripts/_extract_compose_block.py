"""从 compose 文件里切出单个服务块。

E2E-23 引入：多个守卫脚本都需要"取某个服务的 environment 段"，
原先各自内嵌 python heredoc，在 `$(...)` 里嵌套 heredoc 会吞掉
stdin（实测退出码 49，无任何输出），排查浪费了不少时间。
故抽成独立文件，用法：`python3 _extract_compose_block.py <compose> <service>`。

按两空格缩进切块 —— compose 的服务名固定是两个空格的缩进。
刻意不用 YAML 解析器：守卫脚本要在"文件本身可能写错"时也能跑，
而 YAML 解析器遇到语法错误会直接抛异常，那时我们恰恰需要看到服务块。
"""

import re
import sys


def extract(lines, service):
    """返回 service 的块文本；找不到返回 None。"""
    needle = service + ":"
    start = None
    for i, line in enumerate(lines):
        if line.strip() == needle:
            start = i
            break
    if start is None:
        return None

    end = len(lines)
    for j in range(start + 1, len(lines)):
        if re.match(r"^  [a-zA-Z]", lines[j]):
            end = j
            break
    return "\n".join(lines[start:end])


def main():
    if len(sys.argv) != 3:
        print("usage: _extract_compose_block.py <compose-file> <service-name>", file=sys.stderr)
        return 2

    path, service = sys.argv[1], sys.argv[2]
    with open(path, encoding="utf-8") as fh:
        lines = fh.read().split("\n")

    block = extract(lines, service)
    if block is None:
        print("BLOCK_NOT_FOUND")
        return 0

    print(block)
    return 0


if __name__ == "__main__":
    sys.exit(main())
