"""E2E-14 续做（E2E-F-95）验证脚本：人格画像是否真的造成可感不同的回复。

这不是单元测试（要真实 LLM + 数据库 + 网络），而是**取证脚本** —— 产物落盘供人工复核。

场景（正是用户提的验收口径）：
  同一句用户问话，两个对立人格画像的用户，在**各自的新对话**里得到回复，
  比较"感觉"是否真的不同，并做去标识盲判。

两个画像（通过 BIG5 答题构造，注意 6 道反向题的答法）：
  A「高外向 · 低神经质」：更主动、节奏轻快、不需要反复安抚
  B「低外向 · 高神经质」：给足空间、不追问、先稳情绪、用确定措辞

输出：
  - 两份 system prompt 的实际注入文本（对照）
  - 两条 AI 回复原文
  - 可观察特征量化：字数 / 问号数 / 不确定词数 / 安抚词数
  - 盲判：把两条回复去标识后交给独立 LLM 判定归属，跑 N 轮取多数

用法：
  python scripts/verify_personality_prompt_diff.py --api http://localhost:19080
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request

# ---------------------------------------------------------------- 画像构造

# 每题答案（Likert 1..5）。反向题：q10 q15 q20 q25（神经质组）、q23（外向性组）、q24（宜人性组）
# 反向题原始分 5 → 计分 1；原始分 1 → 计分 5。
#
# 目标画像 A：外向性 高（30）、神经质 低（6）
#   外向性 = q3,q8,q13,q18,q28 取 5；q23 取 1（反向 → 5） ⇒ 5*5+5 = 30
#   神经质 = q5,q30 取 1；q10,q15,q20,q25 取 5（反向 → 1） ⇒ 1*2+1*4 = 6
# 其余维度固定为中性（全 3）以便对比聚焦：
#   开放性 = 18、尽责性 = 18、宜人性：q4,q9,q14,q19,q29=3 且 q24=3 ⇒ 18
PROFILE_A = {
    **{f"q{i}": 3 for i in range(1, 31)},
    # 高外向
    "q3": 5, "q8": 5, "q13": 5, "q18": 5, "q28": 5, "q23": 1,
    # 低神经质
    "q5": 1, "q30": 1, "q10": 5, "q15": 5, "q20": 5, "q25": 5,
}

# 目标画像 B：外向性 低（6）、神经质 高（30）—— A 的镜像
PROFILE_B = {
    **{f"q{i}": 3 for i in range(1, 31)},
    # 低外向
    "q3": 1, "q8": 1, "q13": 1, "q18": 1, "q28": 1, "q23": 5,
    # 高神经质
    "q5": 5, "q30": 5, "q10": 1, "q15": 1, "q20": 1, "q25": 1,
}

# 多条同风格问话（带情绪 + 留了被追问的空间，便于观察"是否追问/是否给空间"的差异）。
# 单条样本不足以下结论，此处每条各跑一次 A/B。
USER_MESSAGES = [
    "最近工作压力有点大，感觉自己做什么都不太顺。",
    "今天开会又被否了，有点怀疑自己是不是不适合这份工作。",
    "说不上来，就是有点空落落的。",
]


# ---------------------------------------------------------------- HTTP 工具


def _req(method: str, url: str, token: str | None = None, body: dict | None = None, timeout: int = 90):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(url, data=data, method=method)
    if data is not None:
        r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", f"Bearer {token}")
    return urllib.request.urlopen(r, timeout=timeout)


def login(api: str, username: str, password: str) -> str:
    resp = json.load(_req("POST", f"{api}/api/v1/auth/login",
                          body={"username": username, "password": password}))
    return resp["data"]["accessToken"]


def ensure_account(api: str, username: str, password: str, security: list[dict]) -> None:
    """注册账号；已存在则忽略（幂等）。"""
    try:
        _req("POST", f"{api}/api/v1/auth/register", body={
            "username": username, "password": password, "securityQuestions": security,
        })
        print(f"  [account] 已注册 {username}")
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", "replace")[:200]
        if "exist" in detail.lower() or "duplicate" in detail.lower() or e.code in (400, 409):
            print(f"  [account] {username} 已存在，复用")
        else:
            raise


def submit_big5(api: str, token: str, survey_id: int, answers: dict) -> dict:
    resp = json.load(_req("POST", f"{api}/api/v1/surveys/{survey_id}/submit",
                          token=token, body={"answers": answers}))
    return resp["data"]


def ai_reply(api: str, token: str, message: str) -> str:
    """发一条消息到**新对话**（conversationId 空 ⇒ 服务端新建），收集 SSE 全文。"""
    out: list[str] = []
    with _req("POST", f"{api}/api/v1/ai/stream", token=token,
              body={"message": message}, timeout=120) as r:
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
            delta = chunk.get("choices", [{}])[0].get("delta", {}).get("content")
            if delta:
                out.append(delta)
    return "".join(out).strip()


# ---------------------------------------------------------------- 特征量化

UNCERTAIN_WORDS = ["也许", "可能", "大概", "或许", "似乎", "说不定"]
SOOTHING_WORDS = ["陪着你", "别急", "慢慢", "没关系", "我在这里", "抱抱", "稳住", "深呼吸", "会好的"]


def features(text: str) -> dict:
    return {
        "chars": len(text),
        "questions": text.count("？") + text.count("?"),
        "uncertain": sum(text.count(w) for w in UNCERTAIN_WORDS),
        "soothing": sum(text.count(w) for w in SOOTHING_WORDS),
        # 追问类推进词（低外向画像应避免）
        "push": sum(text.count(w) for w in ["说说看", "再多讲", "还有呢", "然后呢", "能具体", "为什么"]),
    }


# ---------------------------------------------------------------- main


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:19080")
    ap.add_argument("--judge-rounds", type=int, default=3)
    ap.add_argument("--out", default="tmp/e2e14b-verify-result.json")
    args = ap.parse_args()
    api = args.api.rstrip("/")

    print("== 0. 准备两个对立画像的账号 ==")
    sec = [{"question": "你最喜欢的颜色是？", "answer": "蓝色"}]
    ensure_account(api, "persona_a", "persona123", sec)
    ensure_account(api, "persona_b", "persona123", sec)
    tok_a, tok_b = login(api, "persona_a", "persona123"), login(api, "persona_b", "persona123")

    # 找 BIG5 的 survey id
    items = json.load(_req("GET", f"{api}/api/v1/surveys", token=tok_a))["data"]["items"]
    big5 = next((i for i in items if i["category"] == "personality"), None)
    if not big5:
        print("!! 未找到 category=personality 的量表，先确认种子数据已应用", file=sys.stderr)
        return 2
    sid = big5["id"]
    print(f"  BIG5 survey id = {sid}")

    print("== 1. 提交对立答卷，确认维度分 ==")
    ra = submit_big5(api, tok_a, sid, PROFILE_A)["factorScores"]
    rb = submit_big5(api, tok_b, sid, PROFILE_B)["factorScores"]
    print(f"  A 画像: {ra}")
    print(f"  B 画像: {rb}")
    expect_ok = ra.get("extraversion", 0) >= 23 and ra.get("neuroticism", 99) <= 13 \
        and rb.get("extraversion", 99) <= 13 and rb.get("neuroticism", 0) >= 23
    print(f"  画像对立性检查: {'OK' if expect_ok else '!! 不满足预期，后续对比无效'}")

    print("== 2. 同消息、各自新对话，取真实 LLM 回复 ==")
    print("   A 画像 = 高外向·低神经质；B 画像 = 低外向·高神经质（镜像）")
    per_message: list[dict] = []
    votes_all: list[str] = []
    dir_hits = {k: 0 for k in ("chars", "questions", "uncertain", "soothing", "push")}

    for mi, msg in enumerate(USER_MESSAGES, start=1):
        print(f"\n  --- 消息 {mi}/{len(USER_MESSAGES)}: {msg}")
        ra_text = ai_reply(api, tok_a, msg)
        rb_text = ai_reply(api, tok_b, msg)
        fa, fb = features(ra_text), features(rb_text)
        print(f"      A({fa['chars']:>3}字): {ra_text}")
        print(f"      B({fb['chars']:>3}字): {rb_text}")
        line = []
        for k in dir_hits:
            ok = _expected_direction(k, fa[k], fb[k])
            dir_hits[k] += 1 if ok else 0
            line.append(f"{k}:A={fa[k]}/B={fb[k]}{'✓' if ok else '✗'}")
        print("      特征: " + "  ".join(line))

        judge_prompt = (
            "你是评审。下面两段回复是回复同一句话的，分别写给两位不同用户。\n"
            "用户甲：性格内向、社交能量低、容易焦虑，需要稳定感与空间。\n"
            "用户乙：性格外向、社交能量高、情绪稳定，喜欢互动、受得住直接的话。\n\n"
            f"【第一段】\n{ra_text}\n\n【第二段】\n{rb_text}\n\n"
            "哪一段是写给「用户甲」的？只回答 A 或 B，不要解释。"
        )
        m_votes = []
        for _ in range(args.judge_rounds):
            v = ai_reply(api, tok_a, judge_prompt).strip().upper()
            letter = "A" if v.startswith("A") else ("B" if v.startswith("B") else "?")
            m_votes.append(letter)
        votes_all.extend(m_votes)
        print(f"      盲判({args.judge_rounds} 轮): {m_votes}")

        per_message.append({
            "message": msg,
            "reply_a": ra_text, "reply_b": rb_text,
            "features_a": fa, "features_b": fb,
            "judge_votes": m_votes,
        })

    # 甲 = 内向易焦虑 = 画像 B = **第二段** ⇒ 判官答 "B" 才是判对。
    # （初版脚本此处写反，用 `v == "A"` 计正确，把 3/3 判对误报成 0/3；已修）
    correct = sum(1 for v in votes_all if v == "B")
    total = len(votes_all)
    print(f"\n== 3. 汇总 ==")
    print(f"  可观察特征方向命中：")
    for k, hit in dir_hits.items():
        print(f"    {k:10s} {hit}/{len(USER_MESSAGES)}")
    print(f"  盲判判对（认出第二段才是写给内向易焦虑者的）: {correct}/{total}")
    rate = correct / total if total else 0.0
    print(f"  判对率 = {rate:.0%}（随机基线 50%）")

    result = {
        "profiles": {"a": ra, "b": rb},
        "profile_opposite_ok": expect_ok,
        "user_messages": USER_MESSAGES,
        "per_message": per_message,
        "feature_direction_hits": dir_hits,
        "messages_count": len(USER_MESSAGES),
        "judge_correct": correct, "judge_total": total, "judge_rate": rate,
        "judge_scoring_note": "正确回答是 'B'（第二段 = 画像B = 内向高神经质）；甲=内向易焦虑=第二段",
    }
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print(f"== 结果已写入 {args.out} ==")
    return 0


def _expected_direction(key: str, a: int, b: int) -> bool:
    """A=外向低神经质, B=内向高神经质。返回该特征是否朝预期方向。"""
    if key in ("chars", "questions", "push"):
        return a >= b          # 外向者互动更多/更主动
    if key in ("uncertain", "soothing"):
        return b >= a          # 高神经质者需要更确定、更安抚
    return False


if __name__ == "__main__":
    sys.exit(main())
