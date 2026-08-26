// 分布图（圆环/极区等按角度等比渲染的图形）在用量悬殊时，小占比分类几乎不可见。
// 当最大值与最小正值之比超过阈值时，把数值按 log10 等差压缩后再交给图形渲染，
// 让小分类获得可见的扇区角度；未超阈值时原样返回，避免扭曲相对均衡的分布。
// 注意：压缩后的值只用于图形渲染，tooltip 与表格必须继续使用原始数值与真实占比。
const COMPRESS_RATIO_THRESHOLD = 100

export const compressSkewedValues = (values: number[]): number[] => {
  const positives = values.filter((v) => v > 0)
  if (positives.length < 2) return values
  const min = Math.min(...positives)
  const max = Math.max(...positives)
  if (max / min < COMPRESS_RATIO_THRESHOLD) return values
  const minLog = Math.log10(min)
  return values.map((v) => (v > 0 ? Math.log10(v) - minLog + 1 : 0))
}
