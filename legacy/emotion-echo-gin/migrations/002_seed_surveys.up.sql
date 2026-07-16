-- 插入默认心理测验数据
-- 抑郁自评量表 (SDS) - 完整20题

INSERT INTO surveys (id, title, description, estimated_time, questions) VALUES (
    1,
    '抑郁自评量表 (SDS)',
    '用于评估抑郁症状的严重程度，包含20个问题，帮助您了解近期的情绪状态。',
    '5-10分钟',
    '[
        {"id": 1, "title": "我感到情绪沮丧，郁闷", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 2, "title": "我感到早晨心情最好", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 3, "title": "我要哭或想哭", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 4, "title": "我夜间睡眠不好", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 5, "title": "我吃饭像平时一样多", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 6, "title": "我与异性密切接触时和以往一样感到愉快", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 7, "title": "我发觉我的体重在下降", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 8, "title": "我有便秘的苦恼", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 9, "title": "我心跳比平时快", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 10, "title": "我无缘无故地感到疲乏", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 11, "title": "我的头脑跟平常一样清楚", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 12, "title": "我觉得经常做的事情并没有困难", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 13, "title": "我感到不安，心情难以平静", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 14, "title": "我对未来抱有希望", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 15, "title": "我比平常容易生气激动", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 16, "title": "我觉得作出决定是容易的", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 17, "title": "我感到自己是有用的和不可缺少的人", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 18, "title": "我的生活很有意义", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 19, "title": "假若我死了别人会过得更好", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 20, "title": "我仍旧喜爱自己平时喜爱的东西", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]}
    ]'::jsonb
);

-- 焦虑自评量表 (SAS) - 完整20题
INSERT INTO surveys (id, title, description, estimated_time, questions) VALUES (
    2,
    '焦虑自评量表 (SAS)',
    '用于评估焦虑症状的严重程度，包含20个问题，帮助您了解近期的焦虑水平。',
    '5-10分钟',
    '[
        {"id": 1, "title": "我感到紧张或痛苦", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 2, "title": "我感到害怕", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 3, "title": "我感到心烦意乱", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 4, "title": "我感到放松不下来", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 5, "title": "我感到急躁", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 6, "title": "我容易心里烦乱或觉得惊恐", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 7, "title": "我感到手脚发抖打颤", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 8, "title": "我因头痛、颈痛和背痛而烦恼", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 9, "title": "我感到衰弱和疲乏", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 10, "title": "我感到昏昏欲睡", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 11, "title": "我感到心脏跳动得很快", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 12, "title": "我感到头晕", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 13, "title": "我快要晕倒了", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 14, "title": "我感到呼吸困难", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 15, "title": "我感到手脚麻木和刺痛", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 16, "title": "我感到胃痛", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 17, "title": "我感到尿频", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 18, "title": "我感到脸发烧发红", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]},
        {"id": 19, "title": "我容易入睡并且一夜睡得很好", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 4}, {"id": 2, "text": "小部分时间", "score": 3}, {"id": 3, "text": "相当多时间", "score": 2}, {"id": 4, "text": "绝大部分或全部时间", "score": 1}]},
        {"id": 20, "title": "我做噩梦", "type": "radio", "options": [{"id": 1, "text": "没有或很少时间", "score": 1}, {"id": 2, "text": "小部分时间", "score": 2}, {"id": 3, "text": "相当多时间", "score": 3}, {"id": 4, "text": "绝大部分或全部时间", "score": 4}]}
    ]'::jsonb
);
