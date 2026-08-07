-- 为分组增加完整的客户端文本协议准入集合，并按升级前的实际路由行为回填。
-- 旧 allow_messages_dispatch 列在兼容窗口内保留，供旧版本二进制滚动共存。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allowed_client_protocols JSONB;

UPDATE groups
SET allowed_client_protocols = CASE platform
    WHEN 'anthropic' THEN '["anthropic_messages","openai_responses","openai_chat_completions"]'::jsonb
    WHEN 'openai' THEN CASE
        WHEN allow_messages_dispatch THEN '["anthropic_messages","openai_responses","openai_chat_completions"]'::jsonb
        ELSE '["openai_responses","openai_chat_completions"]'::jsonb
    END
    WHEN 'gemini' THEN '["anthropic_messages","openai_responses","openai_chat_completions","gemini_generate_content"]'::jsonb
    WHEN 'antigravity' THEN '["anthropic_messages","openai_responses","openai_chat_completions","gemini_generate_content"]'::jsonb
    WHEN 'qoder' THEN '["anthropic_messages","openai_responses","openai_chat_completions"]'::jsonb
    WHEN 'grok' THEN '["anthropic_messages","openai_responses","openai_chat_completions"]'::jsonb
    ELSE '[]'::jsonb
END
WHERE allowed_client_protocols IS NULL;

ALTER TABLE groups
    ALTER COLUMN allowed_client_protocols SET DEFAULT '[]'::jsonb,
    ALTER COLUMN allowed_client_protocols SET NOT NULL;
