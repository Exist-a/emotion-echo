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
