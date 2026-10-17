#!/usr/bin/env node
import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import process from 'node:process'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..')
const roots = ['shell/src', 'plugins', 'packages']
const extensions = new Set(['.ts', '.tsx', '.vue', '.js', '.mjs'])
const allowed = new Set([
  'packages/api-client/src/index.ts',
  'shell/src/stores/authStore.ts',
  'shell/src/core/plugin-loader/PluginLoader.ts',
])

async function walk(relative) {
  const entries = await readdir(path.join(root, relative), { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const child = path.join(relative, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'node_modules' || entry.name === 'dist') continue
      files.push(...await walk(child))
    } else if (extensions.has(path.extname(entry.name)) && !/\.(test|spec)\.[^.]+$/.test(entry.name)) {
      files.push(child)
    }
  }
  return files
}

const violations = []
for (const base of roots) {
  for (const file of await walk(base)) {
    if (allowed.has(file)) continue
    const text = await readFile(path.join(root, file), 'utf8')
    const lines = text.split('\n')
    for (let index = 0; index < lines.length; index += 1) {
      if (/(^|[^.\w])fetch\s*\(/.test(lines[index])) {
        violations.push(`${file}:${index + 1}: direct fetch outside @hnb/api-client`)
      }
    }
  }
}

if (violations.length > 0) {
  console.error(violations.join('\n'))
  process.exit(1)
}
console.log('API boundary check passed: business requests use @hnb/api-client')
