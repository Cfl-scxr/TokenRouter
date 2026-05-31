# 数据共享快照最终化性能优化计划

## 目标

聚焦优化数据共享采集的快照最终化阶段，优先解决超大会话下 `flush_finalize` P95/最大值异常偏高的问题，不改动已经足够快的采集排队、解析、缓冲合并、DB 查写路径。

## 实施范围

- 将尾部裁剪质量判定从反复完整校验改为线性前缀状态分析，避免超大 messages 下 O(n²)。
- 避免快照最终化后再次对已经 compact 的 messages 执行重复 compact。
- 为大快照场景补充单测与 benchmark，覆盖 partial tail、complete snapshot、超大重复历史。

## 验证

- 运行 `go test ./internal/service` 中数据共享相关测试。
- 运行新增 benchmark，确认大消息量下复杂度不再接近平方增长。

## 非目标

- 本轮不改 DB 写入、payload 压缩、worker pool、buffer 锁粒度、存储模型。
- 本轮不新增 migration。
