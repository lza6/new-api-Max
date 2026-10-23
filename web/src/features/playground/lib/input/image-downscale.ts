/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// 参考图上传前的降采样（T4）。
//
// ## 为什么必须做
//
// 参考图以 **base64 内联**发给上游（`blobUrlToDataUrl` 的既有契约：上游不必回访
// 客户端）。base64 会把体积放大 ~33%，而图像 token 数又近似与像素数成正比——
// 一张手机原图（4000×3000、约 5MB）内联后是 ~6.7MB 字符串，既显著推高**用户账单**，
// 也常见被上游以体积/尺寸超限直接拒绝。
//
// 参考项目的做法（`AGENT_IMG_MAX_DIM=1568` / `AGENT_IMG_MAX_BYTES=1MB`，注释明写
// 「避免上游 base64 token 爆炸」）验证了这条经验；这里取其上限值：1568 是主流
// 多模态模型的长边输入上限（再大也会被上游自己缩放），超过它只是白付 token。
//
// ## 分层
//
// 尺寸计算是**纯函数**（`computeDownscaleSize`），可被单测穷举边界；
// 真正的 canvas 绘制放在 `downscaleImageFile`，只在浏览器里跑。

/** 长边上限（像素）。主流多模态模型的有效输入长边。 */
export const IMAGE_MAX_DIMENSION = 1568

/** 压缩后目标字节上限（1MB）。 */
const IMAGE_MAX_BYTES = 1024 * 1024

/** JPEG 质量阶梯：从高到低逐档尝试，直到压进目标体积。 */
const IMAGE_QUALITY_STEPS = [0.92, 0.85, 0.75, 0.65, 0.55]

/**
 * 计算降采样后的尺寸（纯函数）。
 *
 * 契约：
 *  - **只缩不放**：原图长边已在上限内时原样返回（放大只会凭空增加字节，
 *    且不会增加信息量）。
 *  - 保持宽高比，四舍五入，且**宽高最小为 1**（极小图或极端比例下不能算出 0，
 *    否则 canvas 会抛错）。
 *  - 非法尺寸（0/负数/NaN）返回 `null`，由调用方跳过缩放——不猜一个尺寸出来。
 */
export function computeDownscaleSize(
  width: number,
  height: number,
  maxDimension: number = IMAGE_MAX_DIMENSION
): { width: number; height: number } | null {
  if (
    !Number.isFinite(width) ||
    !Number.isFinite(height) ||
    width <= 0 ||
    height <= 0
  ) {
    return null
  }
  if (!Number.isFinite(maxDimension) || maxDimension <= 0) {
    return null
  }
  const longest = Math.max(width, height)
  if (longest <= maxDimension) {
    return { width: Math.round(width), height: Math.round(height) }
  }
  const scale = maxDimension / longest
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  }
}

/** 该文件类型是否可交给 canvas 处理（canvas 不能解 HEIC/SVG 等）。 */
export function isCanvasDecodableType(type: string): boolean {
  return ['image/jpeg', 'image/png', 'image/webp', 'image/gif', 'image/bmp'].includes(
    type.toLowerCase()
  )
}

/**
 * 把图片文件降采样到 `IMAGE_MAX_DIMENSION` 内并压进 `IMAGE_MAX_BYTES`。
 *
 * **fail-open**：任何环节失败（canvas 不可用、解码失败、类型不支持）都原样返回
 * 原文件——上传失败比上传一张大图更糟，体积问题不应阻断用户。
 *
 * 返回 `{ file, downscaled }`：`downscaled=false` 表示未改动（原图已足够小或
 * 处理失败），调用方据此决定是否提示用户。
 */
export async function downscaleImageFile(
  file: File
): Promise<{ file: File; downscaled: boolean }> {
  const original = { file, downscaled: false }
  if (!isCanvasDecodableType(file.type)) {
    return original
  }
  // 已经小于目标体积且尺寸未知时也无从判断，直接交给后续流程；
  // 这里只按体积短路显然不够（小体积也可能超长边），所以仍走解码。
  if (typeof createImageBitmap !== 'function') {
    return original
  }

  try {
    const bitmap = await createImageBitmap(file)
    try {
      const size = computeDownscaleSize(bitmap.width, bitmap.height)
      if (!size) {
        return original
      }

      const needsResize =
        size.width !== bitmap.width || size.height !== bitmap.height
      // 尺寸未超限且原图已在目标体积内：无需重编码（重编码会损失画质且无收益）。
      if (!needsResize && file.size <= IMAGE_MAX_BYTES) {
        return original
      }

      const canvas = document.createElement('canvas')
      canvas.width = size.width
      canvas.height = size.height
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        return original
      }
      ctx.drawImage(bitmap, 0, 0, size.width, size.height)

      // 逐档降质量，取第一个压进上限的结果；全都不行则用最后一档（已是最小）。
      let blob: Blob | null = null
      for (const quality of IMAGE_QUALITY_STEPS) {
        blob = await new Promise<Blob | null>((resolve) => {
          canvas.toBlob((b) => resolve(b), 'image/jpeg', quality)
        })
        if (blob && blob.size <= IMAGE_MAX_BYTES) {
          break
        }
      }
      if (!blob) {
        return original
      }

      // 压完反而更大（原图本就是高压缩小图）时不替换。
      if (blob.size >= file.size) {
        return original
      }

      const name = file.name.replace(/\.[^.]+$/, '') || 'image'
      return {
        file: new File([blob], `${name}.jpg`, { type: 'image/jpeg' }),
        downscaled: true,
      }
    } finally {
      bitmap.close()
    }
  } catch {
    // 解码/绘制失败：原样上传（fail-open）。
    return original
  }
}
