/**
 * 时间转换工具：ISO ↔ datetime-local 本地格式。
 * datetime-local input 要求 `yyyy-MM-ddThh:mm`（本地时区、无秒/时区后缀），
 * 直接绑定带 Z 的 ISO 会触发浏览器格式警告。
 */

/** ISO → datetime-local 本地格式（yyyy-MM-ddTHH:mm） */
export function isoToLocalInput(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** datetime-local 本地值 → ISO（按本地时区解析） */
export function localInputToIso(value: string): string {
  if (!value) return ''
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}
