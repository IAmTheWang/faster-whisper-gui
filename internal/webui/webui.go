// Package webui embeds the built frontend so cmd/server can serve it from
// a single executable with no separate deploy step.
//
// dist/ is populated by running `bun run build` inside web/ (vite.config.ts
// points its build.outDir here) — see the repository README for the exact
// build order. A placeholder dist/index.html ships in the repo so `go
// build` succeeds even before the frontend has been built once, since
// //go:embed requires the target directory to contain at least one file.
package webui

import "embed"

//go:embed dist
var Dist embed.FS
