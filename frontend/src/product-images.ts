// Stable across pages and reloads, including imported Olist product IDs.
export function sampleProductImage(productId: string) {
  let hash = 0
  for (const char of productId) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) >>> 0
  return `/images/products/sample-${hash % 5 + 1}.webp`
}
