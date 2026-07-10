// imagePricingPlatforms 列出支持媒体定价控制的分组平台。
export const imagePricingPlatforms = new Set([
  "antigravity",
  "gemini",
  "grok",
  "openai",
]);

// supportsImagePricingPlatform 判断指定平台是否展示媒体定价控制。
export const supportsImagePricingPlatform = (platform: string): boolean =>
  imagePricingPlatforms.has(platform);
