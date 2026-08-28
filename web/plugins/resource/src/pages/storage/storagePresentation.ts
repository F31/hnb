export type CapacityStatus = 'Known' | 'Elastic' | 'Unknown' | 'NotReported'

export function formatCapacity(status: CapacityStatus, value?: number): string {
  if (status !== 'Known' || value === undefined) return status
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let amount = value
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024
    unit += 1
  }
  return `${new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(amount)} ${units[unit]}`
}
