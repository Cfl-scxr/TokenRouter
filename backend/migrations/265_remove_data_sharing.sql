-- 数据共享功能已移除：先清理导出元数据，再删除完整会话载荷。
DROP TABLE IF EXISTS data_share_export_artifacts;
DROP TABLE IF EXISTS data_share_sessions;

DROP INDEX IF EXISTS idx_groups_data_sharing_enabled;
ALTER TABLE IF EXISTS groups
    DROP COLUMN IF EXISTS data_sharing_enabled;

DROP INDEX IF EXISTS idx_api_keys_data_sharing_confirmed_group_id;
ALTER TABLE IF EXISTS api_keys
    DROP COLUMN IF EXISTS data_sharing_notice_version,
    DROP COLUMN IF EXISTS data_sharing_confirmed_group_id,
    DROP COLUMN IF EXISTS data_sharing_confirmed_at;

ALTER TABLE IF EXISTS api_key_composite_groups
    DROP COLUMN IF EXISTS data_sharing_notice_version,
    DROP COLUMN IF EXISTS data_sharing_confirmed_at;

-- 仅移除数据共享专属设置，保留所有其他运行设置。
DELETE FROM settings
WHERE key IN (
    'data_sharing_enabled',
    'data_sharing_notice_content',
    'data_sharing_notice_version',
    'data_sharing_capture_skip_rules',
    'data_sharing_export_ticket_key',
    'data_sharing_export_remote_config',
    'data_sharing_export_remote_prefix',
    'data_sharing_storage_limit_bytes',
    'data_sharing_capture_runtime'
);

-- 历史非法 JSON 由运维后续处理，不能阻断破坏性迁移。
DO $$
BEGIN
    BEGIN
        UPDATE settings
        SET value = (value::jsonb - 'include_data_share_sessions')::text,
            updated_at = NOW()
        WHERE key = 'backup_content_config'
          AND jsonb_typeof(value::jsonb) = 'object';
    EXCEPTION
        WHEN invalid_text_representation THEN
            RAISE NOTICE 'skip invalid backup_content_config JSON while removing data sharing';
    END;
END
$$;
