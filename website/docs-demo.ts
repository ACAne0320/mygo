// The previews of native UI's docs: docs-demo/ is a Go app of native UI,
// which this plugin compiles to WebAssembly and publishes at /docs-demo,
// with Go's loader and the page loading it. A page of docs/ui shows one of
// its demos in a frame where it has `<!-- preview name height -->`
// (markdown.server.ts).
import { execFile } from "node:child_process"
import crypto from "node:crypto"
import fs from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { promisify } from "node:util"
import type { Plugin } from "vite"

const run = promisify(execFile)

interface DemoFiles {
  /** The files by their path under /docs-demo/. */
  files: Map<string, Buffer | string>
}

/** Builds the app for browsers. Go must be installed: the build fails without it. */
export async function buildDocsDemo(repoDir: string): Promise<DemoFiles> {
  const out = path.join(await fs.mkdtemp(path.join(os.tmpdir(), "docs-demo-")), "docs-demo.wasm")
  const env = { ...process.env, GOOS: "js", GOARCH: "wasm", CGO_ENABLED: "0" }
  try {
    await run("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", out, "./website/docs-demo"], { cwd: repoDir, env, maxBuffer: 1 << 24 })
  } catch (error) {
    const e = error as { stderr?: string; message: string }
    throw new Error(`docs-demo: go build failed: ${e.stderr || e.message}`)
  }
  const wasm = await fs.readFile(out)
  await fs.rm(path.dirname(out), { recursive: true, force: true })
  const { stdout: goroot } = await run("go", ["env", "GOROOT"], { cwd: repoDir })
  const loader = await fs.readFile(path.join(goroot.trim(), "lib", "wasm", "wasm_exec.js"), "utf8")
  // Named by its content, so that browsers keep it as long as it lasts.
  const name = `docs-demo.${crypto.createHash("sha256").update(wasm).digest("hex").slice(0, 12)}.wasm`
  const page = (await fs.readFile(path.join(repoDir, "website", "docs-demo", "index.html"), "utf8")).replace("__WASM__", name)
  return {
    files: new Map<string, Buffer | string>([
      ["index.html", page],
      ["wasm_exec.js", loader],
      [name, wasm],
    ]),
  }
}

export function docsDemo(repoDir: string): Plugin {
  // The development server builds the app when its page is asked for, so
  // that a reload of a docs page shows the demos as edited.
  let dev: Promise<DemoFiles> | undefined
  return {
    name: "mygo-docs-demo",
    applyToEnvironment: (environment) => environment.name === "client",
    async generateBundle() {
      const { files } = await buildDocsDemo(repoDir)
      for (const [file, source] of files) {
        this.emitFile({ type: "asset", fileName: `docs-demo/${file}`, source })
      }
    },
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const pathname = req.url?.split("?", 1)[0] ?? ""
        const m = /^\/docs-demo(?:\/(.*))?$/.exec(pathname)
        if (!m) return next()
        const file = m[1] || "index.html"
        try {
          if (file === "index.html" || !dev) dev = buildDocsDemo(repoDir)
          const source = (await dev).files.get(file)
          if (source === undefined) return next()
          const types: Record<string, string> = {
            html: "text/html; charset=utf-8",
            js: "text/javascript; charset=utf-8",
            wasm: "application/wasm",
          }
          res.setHeader("Content-Type", types[file.split(".").pop() ?? ""] ?? "application/octet-stream")
          res.end(source)
        } catch (error) {
          next(error)
        }
      })
    },
  }
}
