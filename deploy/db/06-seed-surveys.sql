-- 06-seed-surveys.sql
-- E2E-13: 种子数据 — PHQ-9（抑郁）+ GAD-7（焦虑）
--
-- questions JSONB 格式为 map[string]Question（与 model.JSONMap 兼容）：
--   key = "q1"~"qN"（即 scorer answers map 的 key）
--   value = {title, type, options: [{id, text, score}]}
--   前端需要数组，由 BFF survey_handler 做 map→slice 转换
--
-- scoring_rules JSONB 包含严重程度阈值

-- PHQ-9 (Patient Health Questionnaire-9, 抑郁筛查)
INSERT INTO emotion_echo_assessment.surveys (code, title, description, category, questions, scoring_rules, version, status)
VALUES (
    'PHQ-9',
    'PHQ-9 抑郁症筛查量表',
    '过去两周内，以下问题困扰你的频率是多少？',
    'depression',
    '{"q1":{"title":"做事时提不起劲或没有兴趣","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q2":{"title":"感到心情低落、沮丧或绝望","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q3":{"title":"入睡困难、睡不安稳或睡眠过多","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q4":{"title":"感觉疲倦或没有活力","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q5":{"title":"食欲不振或吃太多","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q6":{"title":"觉得自己很糟或失败","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q7":{"title":"对事物专注有困难","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q8":{"title":"动作或说话速度缓慢或烦躁不安","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q9":{"title":"有不如死掉或伤害自己的念头","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]}}'::jsonb,
    '{"thresholds":[{"min":0,"max":4,"level":"none"},{"min":5,"max":9,"level":"mild"},{"min":10,"max":14,"level":"moderate"},{"min":15,"max":19,"level":"severe"},{"min":20,"max":27,"level":"extreme"}]}'::jsonb,
    1,
    1
) ON CONFLICT (code) DO UPDATE SET
    title = EXCLUDED.title,
    questions = EXCLUDED.questions,
    scoring_rules = EXCLUDED.scoring_rules,
    updated_at = NOW();

-- GAD-7 (Generalized Anxiety Disorder-7, 焦虑筛查)
INSERT INTO emotion_echo_assessment.surveys (code, title, description, category, questions, scoring_rules, version, status)
VALUES (
    'GAD-7',
    'GAD-7 广泛性焦虑量表',
    '过去两周内，以下问题困扰你的频率是多少？',
    'anxiety',
    '{"q1":{"title":"感觉紧张、焦虑或急切","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q2":{"title":"不能够停止或控制担忧","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q3":{"title":"对各种各样的事情担忧过多","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q4":{"title":"很难放松下来","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q5":{"title":"由于不安而无法静坐","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q6":{"title":"变得容易烦恼或急躁","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]},"q7":{"title":"感到似乎将有可怕的事情发生","type":"radio","options":[{"id":1,"text":"完全没有","score":0},{"id":2,"text":"好几天","score":1},{"id":3,"text":"一半以上的天数","score":2},{"id":4,"text":"几乎每天","score":3}]}}'::jsonb,
    '{"thresholds":[{"min":0,"max":4,"level":"none"},{"min":5,"max":9,"level":"mild"},{"min":10,"max":14,"level":"moderate"},{"min":15,"max":21,"level":"severe"}]}'::jsonb,
    1,
    1
) ON CONFLICT (code) DO UPDATE SET
    title = EXCLUDED.title,
    questions = EXCLUDED.questions,
    scoring_rules = EXCLUDED.scoring_rules,
    updated_at = NOW();

-- BIG5 (NEO Five-Factor Inventory-30, 人格五因素量表)
-- 30 题，5 维度（开放性/尽责性/外向性/宜人性/神经质），每维度 6 题
-- Likert 5 点计分：1=非常不同意, 2=不同意, 3=中立, 4=同意, 5=非常同意
-- 部分题目反向计分（在评分器中处理）
INSERT INTO emotion_echo_assessment.surveys (code, title, description, category, questions, scoring_rules, version, status)
VALUES (
    'BIG5',
    '人格五因素量表',
    '请根据您的实际情况，选择最符合您描述的选项。本量表评估您的五大人格特质。',
    'personality',
    '{
        "q1":{"title":"我喜欢尝试新事物和新体验","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q2":{"title":"我做事有条理，喜欢按计划进行","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q3":{"title":"我喜欢参加社交活动，与人交往","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q4":{"title":"我通常信任他人，认为他们是善意的","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q5":{"title":"我经常感到焦虑或担心","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q6":{"title":"我对艺术和美学有浓厚兴趣","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q7":{"title":"我总是按时完成任务","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q8":{"title":"我喜欢成为众人关注的焦点","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q9":{"title":"我容易原谅他人的过错","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q10":{"title":"我情绪稳定，不容易波动","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q11":{"title":"我对抽象概念和理论感兴趣","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q12":{"title":"我做事认真负责","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q13":{"title":"我喜欢热闹，善于言辞","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q14":{"title":"我乐于助人，关心他人","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q15":{"title":"我容易感到沮丧","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q16":{"title":"我喜欢探索新的思想和观念","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q17":{"title":"我做事有始有终","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q18":{"title":"我喜欢与人合作","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q19":{"title":"我对人宽容，不记仇","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q20":{"title":"我很少感到紧张或不安","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q21":{"title":"我喜欢想象和幻想","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q22":{"title":"我注重细节，追求完美","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q23":{"title":"我喜欢独处，享受安静","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q24":{"title":"我容易与人发生冲突","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q25":{"title":"我经常感到快乐和满足","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q26":{"title":"我对新奇的事物充满好奇","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q27":{"title":"我做事有计划，不冲动","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q28":{"title":"我喜欢冒险和刺激","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q29":{"title":"我尊重他人的意见","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]},
        "q30":{"title":"我能很好地控制自己的情绪","type":"radio","options":[{"id":1,"text":"非常不同意","score":1},{"id":2,"text":"不同意","score":2},{"id":3,"text":"中立","score":3},{"id":4,"text":"同意","score":4},{"id":5,"text":"非常同意","score":5}]}
    }'::jsonb,
    '{"dimensions":["openness","conscientiousness","extraversion","agreeableness","neuroticism"],"reverse":["q10","q15","q20","q23","q24","q25"]}'::jsonb,
    1,
    1
) ON CONFLICT (code) DO UPDATE SET
    title = EXCLUDED.title,
    questions = EXCLUDED.questions,
    scoring_rules = EXCLUDED.scoring_rules,
    updated_at = NOW();
