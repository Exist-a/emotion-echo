-- 查找重复的消息
-- 查找 conversation_id、sender 和 content 都相同的多条记录

-- 首先查看重复消息的情况
SELECT 
    conversation_id,
    sender,
    content,
    COUNT(*) as duplicate_count,
    ARRAY_AGG(id ORDER BY created_at) as message_ids,
    MIN(created_at) as earliest,
    MAX(created_at) as latest
FROM messages
WHERE sender = 'user'
GROUP BY conversation_id, sender, content
HAVING COUNT(*) > 1
ORDER BY conversation_id, latest DESC;

-- 删除重复消息，保留最早的那条
-- 使用 CTID（行号）来删除重复记录
DELETE FROM messages
WHERE id IN (
    SELECT id FROM (
        SELECT 
            id,
            ROW_NUMBER() OVER (PARTITION BY conversation_id, sender, content ORDER BY created_at ASC) as rn
        FROM messages
        WHERE sender = 'user'
    ) sub
    WHERE rn > 1
);

-- 验证清理结果
SELECT conversation_id, sender, content, COUNT(*) 
FROM messages 
WHERE sender = 'user' 
GROUP BY conversation_id, sender, content 
HAVING COUNT(*) > 1;
