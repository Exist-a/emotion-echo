"""Stage 82 · 意图分类 + 按意图回复风格（llm-chat-real-pipeline PR-3a）

6 类意图（docs/plans/intent-classification-6-types.md 定义复用）：
  emotional_support 情感疏导 / study_help 学习问题 / tech_help 技术问题 /
  career_help 职业问题 / lifestyle 生活问题 / other 其他

规则式关键词打分（与 main.analyze 的情绪分析同款风格）——确定性、可单测、
无 LLM 依赖；风格指令取自 docs/plans/ai-response-structured.md §阶段 2。
"""
import logging

logger = logging.getLogger(__name__)

INTENTS = {
    "emotional_support",
    "study_help",
    "tech_help",
    "career_help",
    "lifestyle",
    "other",
}

# 关键词规则（中文为主，辅以常见英文）。命中数最多者胜出；并列/零命中 → other。
INTENT_KEYWORDS: dict[str, list[str]] = {
    "emotional_support": [
        "难过", "伤心", "低落", "沮丧", "崩溃", "撑不下去", "想哭", "眼泪",
        "压力大", "压力好大", "焦虑", "失眠", "睡不着", "emo", "安慰",
        "心情不好", "不开心", "孤独", "空虚", "迷茫", "想找人聊聊", "陪伴",
    ],
    "study_help": [
        "作业", "题目", "习题", "复习", "考试", "期末", "预习", "学习方法",
        "背书", "记不住", "论文", "答辩", "做题", "数学", "英语单词", "gpa",
        "挂科", "补考", "教我",
    ],
    "tech_help": [
        "代码", "报错", "bug", "调试", "python", "java", "golang", "javascript",
        "程序", "编译", "部署", "服务器", "数据库", "sql", "接口", "api",
        "框架", "算法", "运行不了", "跑不起来", "异常", "堆栈", "报错了",
    ],
    "career_help": [
        "面试", "简历", "offer", "跳槽", "离职", "入职", "职业规划", "升职",
        "加薪", "职场", "领导", "同事关系", "工作压力", "裁员", "转行",
        "岗位", "招聘", "求职",
    ],
    "lifestyle": [
        "电影", "电视剧", "综艺", "游戏", "旅游", "旅行", "美食", "做饭",
        "健身", "运动", "跑步", "爱好", "推荐", "周末", "假期", "放松",
        "逛街", "购物", "音乐", "读书", "小说",
    ],
}

# 回复风格指令（docs/plans/ai-response-structured.md §阶段 2 的每意图 prompt）
STYLE_INSTRUCTIONS: dict[str, str] = {
    "emotional_support": (
        "回复要求：1.像朋友一样自然对话，语气温暖共情；"
        "2.不要用序号或列表，保持流畅自然；3.适当使用换行分段；"
        "4.体现理解和支持，先接住情绪再轻引导。"
    ),
    "study_help": (
        "回复要求：1.用清晰的步骤说明（1. 2. 3.）；"
        "2.关键点可以加粗（**重点**）；3.结尾可以加鼓励的话；4.保持温和耐心。"
    ),
    "tech_help": (
        "回复要求：1.步骤清晰，用序号列出；"
        "2.需要代码时，用代码块包裹（```）；3.语言简洁专业；4.适当使用无序列表。"
    ),
    "career_help": (
        "回复要求：1.分点给出建议（1. 2. 3.）；2.逻辑清晰，条理分明；"
        "3.关键建议加粗标注。"
    ),
    "lifestyle": (
        "回复要求：1.轻松有帮助的小建议；2.适当用列表或分段；3.语气轻快。"
    ),
    "other": (
        "回复要求：1.简洁直接；2.适当用列表或分段。"
    ),
}


def classify_intent(text: str) -> tuple[str, float]:
    """规则式意图分类。返回 (intent, confidence)。

    打分：命中关键词数最多者胜出；零命中或并列无法区分 → other。
    confidence = 命中数 / (命中数 + 3)，平滑到 (0, 1) 区间（零命中为 0）。
    """
    if not text or not text.strip():
        return "other", 0.0

    lowered = text.lower()
    scores: dict[str, int] = {}
    for intent, keywords in INTENT_KEYWORDS.items():
        scores[intent] = sum(1 for kw in keywords if kw in lowered)

    best = max(scores, key=lambda k: scores[k])
    best_score = scores[best]
    if best_score == 0:
        return "other", 0.0

    # 并列检查：唯一冠军才算高置信，否则并入 other（避免误路由）
    ties = [k for k, v in scores.items() if v == best_score]
    if len(ties) > 1:
        return "other", 0.0

    confidence = best_score / (best_score + 3)
    return best, round(confidence, 2)


def style_instruction(intent: str) -> str:
    """按意图取回复风格指令；未知意图按 other 处理"""
    return STYLE_INSTRUCTIONS.get(intent, STYLE_INSTRUCTIONS["other"])


def inject_style(messages: list[dict], intent: str) -> list[dict]:
    """把风格指令注入消息列表：已有 system 就地扩写，没有则前置。

    不变异入参；返回新列表。
    """
    instruction = style_instruction(intent)
    out = [dict(m) for m in messages]
    for m in out:
        if m.get("role") == "system":
            m["content"] = f"{m['content']}\n{instruction}"
            return out
    return [{"role": "system", "content": instruction}] + out
