#!/usr/bin/env python3
"""
Round 5: 断链修复脚本
策略：把所有 docs/ 内文件里的相对 .md 链接转换成"仓库根相对路径"（以 / 开头）。
- 输入：docs/ 下所有 .md 文件，加上 README.md, AGENTS.md, QUICKSTART.md
- 输出：原地替换 .md 链接

对于"找不到真实目标"的链接，保留原样并打印到 broken_residual.txt
"""
import os, re

ROOT = '.'

# 已知目标映射（指向 Round 3 后的实际位置）
RENAMES = {
    # architecture
    'architecture-decisions.md': 'docs/architecture/decisions.md',
    'architecture-positioning.md': 'docs/architecture/positioning.md',
    'distributed-architecture.md': 'docs/architecture/distributed.md',
    'distributed-roadmap.md': 'docs/architecture/roadmap.md',
    'architecture-audit-2026-08-31.md': 'docs/architecture/audit-2026-08-31.md',
    'microservice-decomposition-plan.md': 'docs/architecture/decomposition-plan.md',
    'microservices-architecture.md': 'docs/architecture/microservices.md',
    # ADR
    'adr-2026-09-chart-contract-alignment.md': 'docs/architecture/adr/adr-2026-09-chart-contract-alignment.md',
    'adr-2026-09-incremental-rpc-adoption.md': 'docs/architecture/adr/adr-2026-09-incremental-rpc-adoption.md',
    'adr-2026-09-known-gaps-patch-fu.md': 'docs/architecture/adr/adr-2026-09-known-gaps-patch-fu.md',
    'adr-2026-09-known-gaps.md': 'docs/architecture/adr/adr-2026-09-known-gaps.md',
    'adr-2026-09-llm-fusion-hardening.md': 'docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md',
    'adr-2026-09-nacos-reintroduction.md': 'docs/architecture/adr/adr-2026-09-nacos-reintroduction.md',
    # deployment
    'git-layout.md': 'docs/deployment/git-layout.md',
    'deploy/README.md': 'docs/deployment/docker-compose.md',
    # ai-models
    'ai-images-build-guide.md': 'docs/ai-models/build-guide.md',
    'xtts-cloud-api-decision.md': 'docs/ai-models/xtts-decision.md',
    'xtts-cloud-api-integration.md': 'docs/ai-models/xtts-integration.md',
    # frontend
    'Emotion-Echo-Web/docs/DESIGN.md': 'docs/frontend/design.md',
}

# 收集所有 stage-XX.md 的真实路径
def collect_stage_targets():
    stages_dir = os.path.join(ROOT, 'docs', 'stages')
    targets = {}
    for f in os.listdir(stages_dir):
        if f.endswith('.md'):
            targets[f] = f'docs/stages/{f}'
    return targets

STAGE_TARGETS = collect_stage_targets()


def resolve_link(link):
    """把单个相对 .md 链接解析为仓库根相对路径（以 / 开头）。"""
    # 跳过 file:// 协议
    if link.startswith('file://'):
        return None  # 无法可靠解析，保留
    # 跳过 http(s)
    if link.startswith('http'):
        return None
    # 跳过绝对路径
    if link.startswith('/'):
        return None  # 已是仓库根相对
    # 去掉 ./ 前缀
    if link.startswith('./'):
        link = link[2:]
    # 跳过 ../docs/xxx 这种从深处指回 docs/
    if link.startswith('../docs/'):
        rest = link[len('../docs/'):]
        return resolve_link(rest)
    # 跳过 ../AGENTS.md 等
    if link.startswith('../') and not link.startswith('../docs/'):
        return None  # 保留原样
    # 拆分路径
    parts = link.split('/')
    fname = parts[-1]
    # 1) 在 RENAMES 表中
    if link in RENAMES:
        return '/' + RENAMES[link]
    if fname in RENAMES:
        return '/' + RENAMES[fname]
    # 2) stage 文件
    if fname in STAGE_TARGETS:
        return '/' + STAGE_TARGETS[fname]
    # 3) 不识别 —— 保留原样
    return None


def process_file(fp):
    with open(fp, encoding='utf-8') as fh:
        content = fh.read()
    changes = 0

    def repl(m):
        nonlocal changes
        prefix = m.group(1)
        link = m.group(2)
        suffix = m.group(3)
        if not link.endswith('.md'):
            return m.group(0)
        new = resolve_link(link)
        if new is None:
            return m.group(0)
        changes += 1
        return f'{prefix}{new}{suffix}'

    new_content = re.sub(r'(\]\()([^)]+)(\))', repl, content)
    if changes > 0:
        with open(fp, 'w', encoding='utf-8') as fh:
            fh.write(new_content)
    return changes


# 主流程
targets = ['README.md', 'AGENTS.md', 'QUICKSTART.md']
for dirpath, dirs, files in os.walk('docs'):
    if any(x in dirpath for x in ['node_modules', '.git', '.output', '.pytest_cache']):
        continue
    for f in files:
        if f.endswith('.md'):
            targets.append(os.path.join(dirpath, f))

total_changes = 0
for fp in targets:
    c = process_file(fp)
    if c > 0:
        print(f'  {fp}: {c} links rewritten')
        total_changes += c
print(f'\nTotal: {total_changes} links rewritten')
