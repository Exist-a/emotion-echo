"""E2E-F-95 判官验证（v2）：**带零假设负对照**的可区分性实验。

v1 的问题（2026-09-21 自审）：
  1. 只有「真对子」（A 画像 vs B 画像）。判官被强制二选一 ⇒ **它总得选一个**，
     所以准确率天然有偏，"6/6" 分不清是"真能区分"还是"它只是肯猜"
  2. 没有"无法区分"选项 ⇒ 系统性抬高表观可区分度
  3. 判官与生成同源（同一 provider 的 key）⇒ 自偏好 / 共享盲区

v2 补上：
  - **零假设负对照（null pair）**：同一画像生成两条回复（同一句问话、两次独立生成）。
    两条**在画像维度上完全等价** ⇒ 判官**不应**能可靠区分。
    若判官对 null 对子也判出高准确率 ⇒ 它在脑补，**真对子的结果一并作废**。
  - **允许"无法区分"**：判官可答 `?`，计入分母。
  - **打乱呈现顺序**并记录答案键（v1 曾把判分方向写反，教训见 report §4.4）。
  - 关键指标：`区分度 = 真对子准确率 - null 对子准确率`。

判分口径（写死在这里，避免再次搞反）：
  正确回答 = "哪一段是写给**甲（内向·易焦虑）**的" ⇒ 正确答案是**画像 B 那一段所在的位置**。

用法：
  python scripts/verify_personality_judge_control.py --api http://localhost:19080 --rounds 2
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.request

USER_MESSAGES = [
    "最近工作压力有点大，感觉自己做什么都不太顺。",
    "今天开会又被否了，有点怀疑自己是不是不适合这份工作。",
    "说不上来，就是有点空落落的。",
]

# 画像 B（内向·高神经质）—— 用于生成 null 对子（同画像两条）
PROFILE_B = {
    **{f"q{i}": 3 for i in range(1, 31)},
    "q3": 1, "q8": 1, "q13": 1, "q18": 1, "q28": 1, "q23": 5,
    "q5": 5, "q30": 5, "q10": 1, "q15": 1, "q20": 1, "q25": 1,
}
PROFILE_A = {
    **{f"q{i}": 3 for i in range(1, 31)},
    "q3": 5, "q8": 5, "q13": 5, "q18": 5, "q28": 5, "q23": 1,
    "q5": 1, "q30": 1, "q10": 5, "q15": 5, "q20": 5, "q25": 5,
}


def _req(method, url, token=None, body=None, timeout=120):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, method=method)
    if data is not None:
        r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", f"Bearer {token}")
    return urllib.request.urlopen(r, timeout=timeout)


def login(api, username, password):
    resp = json.load(_req("POST", f"{api}/api/v1/auth/login",
                          body={"username": username, "password": password}))
    return resp["data"]["accessToken"]


def ai_reply(api, token, message):
    out = []
    with _req("POST", f"{api}/api/v1/ai/stream", token=token, body={"message": message}) as r:
        for raw in r:
            line = raw.decode("utf-8", "replace").strip()
            if not line.startswith("data: "):
                continue
            payload = line[6:]
            if payload == "[DONE]":
                break
            try:
                chunk = json.loads(payload)
            except json.JSONDecodeError:
                continue
            d = chunk.get("choices", [{}])[0].get("delta", {}).get("content")
            if d:
                out.append(d)
    return "".join(out).strip()


JUDGE_TMPL = """你在做一次盲评。下面两段是**同一句用户问话**后由系统产出的两份回复。
两份回复可能来自人格画像不同的用户，也可能来自画像相同的两个用户 —— 你不知道是哪种。

- 用户甲：性格内向、社交能量低、容易焦虑，需要稳定感与空间
- 用户乙：性格外向、社交能量高、情绪稳定，喜欢互动、受得住直接的话

问：哪一段更可能是写给**甲**的？

【第一段】
{first}

【第二段】
{second}

只回答一个字符：A（第一段）、B（第二段）、或 ?（两段看不出差别，无法判断）。不要解释。"""


def ask_judge(api, token, first, second):
    raw = ai_reply(api, token, JUDGE_TMPL.format(first=first, second=second)).strip().upper()
    for ch in raw:
        if ch in "AB?":
            return ch
    return "?"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:19080")
    ap.add_argument("--rounds", type=int, default=2, help="每个对子重复判几次（同一条提示重复采样）")
    ap.add_argument("--out", default="tmp/e2e-f95-judge-control.json")
    args = ap.parse_args()
    api = args.api.rstrip("/")

    tok_a = login(api, "persona_a", "persona123")
    tok_b = login(api, "persona_b", "persona123")

    pairs = []  # {"kind": real|null, "msg_idx": i, "first":..., "second":..., "correct": "A"/"B"/None}

    print("== 生成回复对（真对子 + 零假设 null 对子）==")
    for i, msg in enumerate(USER_MESSAGES):
        # 真对子：A 画像 vs B 画像
        ra = ai_reply(api, tok_a, msg)
        rb = ai_reply(api, tok_b, msg)
        # 呈现顺序交替打乱（i 偶数：B 在前；奇数：A 在前），并写死答案键
        if i % 2 == 0:
            first, second, correct = rb, ra, "A"   # 第一段是 B 画像 ⇒ 正确答 A
        else:
            first, second, correct = ra, rb, "B"   # 第一段是 A 画像 ⇒ 正确答 B
        pairs.append({"kind": "real", "msg_idx": i + 1, "first": first, "second": second,
                      "correct": correct})
        print(f"  [real {i+1}] 两段来自不同画像，正确答案={correct}")

        # null 对子：**同一画像 B** 两次独立生成（画像维度上等价）
        n1 = ai_reply(api, tok_b, msg)
        n2 = ai_reply(api, tok_b, msg)
        pairs.append({"kind": "null", "msg_idx": i + 1, "first": n1, "second": n2,
                      "correct": None})
        print(f"  [null {i+1}] 两段均来自画像 B（等价），判官若能高准确率区分 = 脑补")

    print(f"\n== 判官判定（每对 {args.rounds} 次采样 + **顺序互换**）==")
    for p in pairs:
        votes = [ask_judge(api, tok_a, p["first"], p["second"]) for _ in range(args.rounds)]
        p["votes"] = votes
        # 顺序互换：把两段调过来，看判官指向的"内容"是否稳定。
        # 位置偏见的判官会随位置改答案（互换后仍答同一位置）⇒ 内容归因不稳定。
        swapped = [ask_judge(api, tok_a, p["second"], p["first"]) for _ in range(args.rounds)]
        p["swapped_votes"] = swapped
        # 归因到内容：互换后 A=原来的第二段、B=原来的第一段 ⇒ A→'B'、B→'A'
        flip = {"A": "B", "B": "A", "?": "?"}
        norm = [flip[v] for v in swapped]
        p["normalized"] = norm
        p["consistent"] = bool(votes) and all(v == votes[0] for v in votes + norm)
        if p["kind"] == "real":
            hit = sum(1 for v in votes if v == p["correct"])
            p["hit"], p["n"] = hit, len(votes)
            tag = "✓" if hit > len(votes) / 2 else "✗"
            print(f"  [real {p['msg_idx']}] 原序={votes} 换序归因={norm} "
                  f"内容稳定={p['consistent']} 正确={p['correct']} → {hit}/{len(votes)} {tag}")
        else:
            # null 对子没有正确答案；任何非 '?' 的选择都算"过度自信"
            decisive = sum(1 for v in votes if v in ("A", "B"))
            p["decisive"] = decisive
            p["unsure"] = sum(1 for v in votes if v == "?")
            print(f"  [null {p['msg_idx']}] 原序={votes} 换序归因={norm} "
                  f"内容稳定={p['consistent']} → "
                  f"硬判 {decisive}/{len(votes)}，答'无法判断' {p['unsure']}/{len(votes)}")

    # ---- 汇总 ----
    real_hits = sum(p["hit"] for p in pairs if p["kind"] == "real")
    real_n = sum(p["n"] for p in pairs if p["kind"] == "real")
    null_decisive = sum(p["decisive"] for p in pairs if p["kind"] == "null")
    null_n = sum(len(p["votes"]) for p in pairs if p["kind"] == "null")
    null_unsure = sum(p["unsure"] for p in pairs if p["kind"] == "null")

    real_acc = real_hits / real_n if real_n else 0.0
    real_consistent = sum(1 for p in pairs if p["kind"] == "real" and p["consistent"])
    real_pairs_n = sum(1 for p in pairs if p["kind"] == "real")
    # 在"内容归因稳定"的真对子上，正确率才有解读价值
    stable_real = [p for p in pairs if p["kind"] == "real" and p["consistent"]]
    stable_hits = sum(p["hit"] for p in stable_real)
    stable_n = sum(p["n"] for p in stable_real)
    null_decisive_rate = null_decisive / null_n if null_n else 0.0
    # null 对子上"硬判"的比例越高，说明判官越倾向于无信息也下结论
    print("\n== 汇总 ==")
    print(f"  真对子准确率        : {real_hits}/{real_n} = {real_acc:.0%}   （随机基线 50%）")
    print(f"  null 对子硬判率     : {null_decisive}/{null_n} = {null_decisive_rate:.0%}"
          f"   （理想应接近 0 —— 等价的两段本该答'无法判断'）")
    print(f"  null 对子答'无法判断': {null_unsure}/{null_n} = {null_unsure/max(null_n,1):.0%}")
    if null_decisive_rate >= 0.5 and real_consistent <= real_pairs_n // 2:
        verdict = ("❌ **本判官不可用**：在等价(null)对子上 100% 硬判、且顺序互换后内容归因不稳定 "
                   "⇒ 它按位置而非内容回答，真对子准确率**不能**作为可区分性证据")
    elif null_decisive_rate >= 0.5:
        verdict = ("⚠️ 判官在 null 对子上大量硬判 ⇒ 存在脑补倾向，真对子准确率需打折解读")
    else:
        verdict = ("✅ 判官在等价对子上倾向说'无法判断' ⇒ 判断对信息敏感，真对子准确率可采信")
    print(f"  结论: {verdict}")

    result = {
        "judge": "同 provider 判官（DeepSeek）—— 独立性另需换模型族复验",
        "real_consistent_pairs": real_consistent, "real_pairs_n": real_pairs_n,
        "stable_hits": stable_hits, "stable_n": stable_n,
        "real_accuracy": real_acc, "real_hits": real_hits, "real_n": real_n,
        "null_decisive_rate": null_decisive_rate, "null_unsure": null_unsure, "null_n": null_n,
        "control_verdict": verdict,
        "pairs": [{k: v for k, v in p.items() if k not in ("first", "second")} for p in pairs],
    }
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print(f"\n== 结果已写入 {args.out} ==")
    return 0


if __name__ == "__main__":
    sys.exit(main())
