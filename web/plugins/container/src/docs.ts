/**
 * 文档帮助链接基址（构建/部署时注入 VITE_DOCS_URL）。
 * 未配置时返回空串，调用方应隐藏帮助入口而不是打开占位域名。
 */
export function docsUrl(path = ''): string {
  const base = (import.meta.env.VITE_DOCS_URL as string | undefined)?.replace(/\/+$/, '') ?? ''
  if (!base) return ''
  return path ? `${base}${path.startsWith('/') ? path : `/${path}`}` : base
}
