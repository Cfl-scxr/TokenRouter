ALTER TABLE group_availability_probe_states
    ADD COLUMN IF NOT EXISTS consecutive_failures BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN group_availability_probe_states.consecutive_failures IS
    '当前连续失败探测次数；成功归零，失败递增';

-- 从每个分组最近一次成功之后的失败记录回填，避免升级后把已有故障误显示为 0。
WITH latest_success AS (
    SELECT DISTINCT ON (group_id)
        group_id,
        started_at,
        id
    FROM group_availability_probe_results
    WHERE success = TRUE
    ORDER BY group_id, started_at DESC, id DESC
), failure_counts AS (
    SELECT
        states.group_id,
        COUNT(results.id)::BIGINT AS consecutive_failures
    FROM group_availability_probe_states states
    LEFT JOIN latest_success success
        ON success.group_id = states.group_id
    LEFT JOIN group_availability_probe_results results
        ON results.group_id = states.group_id
       AND results.success = FALSE
       AND (
            success.group_id IS NULL
            OR results.started_at > success.started_at
            OR (results.started_at = success.started_at AND results.id > success.id)
       )
    GROUP BY states.group_id
)
UPDATE group_availability_probe_states states
SET consecutive_failures = failure_counts.consecutive_failures,
    updated_at = NOW()
FROM failure_counts
WHERE states.group_id = failure_counts.group_id;
