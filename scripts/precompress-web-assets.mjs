import { readdir, readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { gzip } from 'node:zlib'
import { promisify } from 'node:util'

const gzipAsync = promisify(gzip)
const repositoryRoot = path.resolve(import.meta.dirname, '..')
const outputDirectory = path.join(repositoryRoot, 'public')
const compressibleExtensions = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.svg'
])

async function listFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const nested = await Promise.all(entries.map((entry) => {
    const entryPath = path.join(directory, entry.name)
    return entry.isDirectory() ? listFiles(entryPath) : [entryPath]
  }))
  return nested.flat()
}

const files = (await listFiles(outputDirectory)).filter((file) =>
  compressibleExtensions.has(path.extname(file))
)

await Promise.all(files.map(async(file) => {
  const source = await readFile(file)
  const compressed = await gzipAsync(source, { level: 9 })
  await writeFile(`${file}.gz`, compressed)
}))

console.log(`Pre-compressed ${files.length} Web assets.`)
