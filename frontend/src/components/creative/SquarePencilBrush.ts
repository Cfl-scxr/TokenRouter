/**
 * 方头画笔：PencilBrush 子类，用于创作台 mask 涂抹的"方"形状。
 *
 * 实现依据（fabric 7.4 源码，node_modules/fabric/dist/index.mjs 中 PencilBrush）：
 * - 预览阶段 PencilBrush._render 在 contextTop 上 _saveAndTransform(ctx) 应用 viewportTransform 后描边；
 *   其 onMouseMove 的增量分支调用的是「类名限定」的静态方法 PencilBrush.drawSegment，
 *   实例覆写无法拦截，因此方头笔必须走全量重绘路径（clearContext + _render）。
 * - 落笔结束 _finalizeAndAddPath 依次调用可覆写的实例方法 convertPointsToSVGPath / createPath 生成最终 Path。
 *
 * 方头笔迹 = 不平滑的直线折线 + strokeLineCap 'butt' + strokeLineJoin 'miter'：
 * 笔画端点与拐角呈方形，与圆头笔（二次贝塞尔平滑 + round 帽）区分。
 * 单点单击生成一条长度等于笔宽的水平线段，butt 帽下恰好渲染为 笔宽×笔宽 的方块；
 * 预览阶段对单点用 fillRect 画同样的方块，保证预览与最终 Path 视觉一致。
 */

import { PencilBrush, type Path, type Point, type TEvent, type TSimplePathData } from 'fabric'

export class SquarePencilBrush extends PencilBrush {
  /**
   * 鼠标移动：沿用父类的点采集与直线修饰键判断，但每次全量重绘方头预览，
   * 避免父类增量分支调用静态 drawSegment 画出不一致的圆头线段。
   */
  override onMouseMove(pointer: Point, options: TEvent): void {
    if (!this.canvas._isMainEvent(options.e)) return
    if (this.limitedToCanvasSize === true && this._isOutSideCanvas(pointer)) return
    if (this._addPoint(pointer)) {
      this.canvas.clearContext(this.canvas.contextTop)
      this._render()
    }
  }

  /**
   * 在 contextTop 上绘制方头预览：单点画方块，其余画 butt/miter 折线。
   * 画笔样式（strokeStyle/lineWidth）由父类 _reset 里的 _setBrushStyles 设置。
   */
  override _render(ctx: CanvasRenderingContext2D = this.canvas.contextTop): void {
    const points = this._points
    if (!points.length) return
    this._saveAndTransform(ctx)
    if (points.length === 1) {
      const half = this.width / 2
      ctx.fillStyle = this.color
      ctx.fillRect(points[0].x - half, points[0].y - half, this.width, this.width)
    } else {
      ctx.lineCap = 'butt'
      ctx.lineJoin = 'miter'
      ctx.beginPath()
      ctx.moveTo(points[0].x, points[0].y)
      for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y)
      }
      ctx.stroke()
    }
    ctx.restore()
  }

  /**
   * 点集 → SVG path：不做曲线平滑，直接输出直线折线；
   * 单点单击输出一条长度等于笔宽的水平线段（配合 butt 帽渲染为方块）。
   */
  override convertPointsToSVGPath(points: Point[]): TSimplePathData {
    if (!points.length) return []
    if (points.length === 1) {
      const half = this.width / 2
      return [
        ['M', points[0].x - half, points[0].y],
        ['L', points[0].x + half, points[0].y],
      ]
    }
    const path: TSimplePathData = [['M', points[0].x, points[0].y]]
    for (let i = 1; i < points.length; i++) {
      path.push(['L', points[i].x, points[i].y])
    }
    return path
  }

  /** 最终 Path 使用方头端帽与斜接拐角，与预览一致 */
  override createPath(pathData: TSimplePathData): Path {
    const path = super.createPath(pathData)
    path.set({ strokeLineCap: 'butt', strokeLineJoin: 'miter' })
    return path
  }
}
