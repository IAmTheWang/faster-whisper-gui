# web/

The frontend: Vite + vanilla TypeScript, deliberately no framework (React/Vue/etc.) given how small the UI is — see [src/CLAUDE.md](src/CLAUDE.md) and [src/components/CLAUDE.md](src/components/CLAUDE.md) for how it's structured without one.

**Package manager is bun, not npm.** `bun.lock` is the committed lockfile; there is no `package-lock.json` (actively gitignored — see root `.gitignore` — in case npm gets run here by mistake and creates a conflicting one). Always run scripts as `bun run <script>` (`bun run dev`, `bun run build`), never the bare `bun build`/`bun install`/`bun run` shorthand for a script that happens to share a name with one of bun's own built-in subcommands — `build` is exactly such a collision here (`package.json`'s `"build": "tsc --noEmit && vite build"` would be shadowed by bun's own bundler if invoked as bare `bun build`).

Key files:

- **`index.html`** — Vite's entry point.
- **`vite.config.ts`** — dev-server proxy target for `/api` reads `process.env.FASTER_WHISPER_GUI_BACKEND_ADDR`, falling back to `127.0.0.1:8080`; `cmd/dev` sets that env var when it spawns this dev server so a custom `-addr` on the Go backend doesn't silently break the proxy. `build.outDir` points to `../internal/webui/dist`, **not** a local `web/dist/` — that's where Go's `go:embed` picks it up (see `internal/webui/CLAUDE.md`).
- **`tsconfig.json`** — includes `"types": ["vite/client"]`, needed for `main.ts`'s side-effect `import './styles.css'` to type-check under `tsc --noEmit` (otherwise TS has no ambient declaration for a bare `.css` import).

Build output never lives in `web/` itself — don't go looking for `web/dist/`, it doesn't exist; check `internal/webui/dist/` instead.
