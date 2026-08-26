import { describe, expect, it } from 'vitest'

import { compressSkewedValues } from '../chartDisplayScale'

describe('compressSkewedValues', () => {
  it('returns values unchanged when the ratio is below the threshold', () => {
    expect(compressSkewedValues([1200, 600])).toEqual([1200, 600])
    expect(compressSkewedValues([99, 1])).toEqual([99, 1])
  })

  it('returns values unchanged when fewer than two positive values exist', () => {
    expect(compressSkewedValues([42])).toEqual([42])
    expect(compressSkewedValues([0, 0])).toEqual([0, 0])
    expect(compressSkewedValues([10, 0])).toEqual([10, 0])
  })

  it('compresses skewed values so the smallest positive stays visible', () => {
    const result = compressSkewedValues([1_000_000, 10_000, 100])
    // log10 压缩后最小值固定为 1，最大值 = log10(比值) + 1
    expect(result[2]).toBe(1)
    expect(result[0]).toBe(5)
    expect(result[1]).toBe(3)
  })

  it('keeps zero values at zero while compressing positives', () => {
    const result = compressSkewedValues([1_000_000, 0, 100])
    expect(result[0]).toBe(5)
    expect(result[1]).toBe(0)
    expect(result[2]).toBe(1)
  })
})
