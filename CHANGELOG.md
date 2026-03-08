# Changelog

## [0.1.2](https://github.com/HerbHall/samverk/compare/v0.1.1...v0.1.2) (2026-03-08)

### Bug Fixes

- **dispatcher:** heuristic fallback when issue has no frontmatter ([#188](https://github.com/HerbHall/samverk/issues/188)) ([67f09f2](https://github.com/HerbHall/samverk/commit/67f09f28836135d135f528f79f7e328be814ba56)), closes [#180](https://github.com/HerbHall/samverk/issues/180)

## [0.1.1](https://github.com/HerbHall/samverk/compare/v0.1.0...v0.1.1) (2026-03-07)

### Features

- add API key management with hash-based auth and per-key scoping ([#119](https://github.com/HerbHall/samverk/issues/119)) ([#128](https://github.com/HerbHall/samverk/issues/128)) ([e57fee6](https://github.com/HerbHall/samverk/commit/e57fee62894b94012e95b759038cf20a9ff01c89))
- add Bearer token auth middleware for MCP endpoint ([#107](https://github.com/HerbHall/samverk/issues/107)) ([bef6881](https://github.com/HerbHall/samverk/commit/bef6881cd18c27558f92affd5ceb5fe250c2caeb))
- add Copilot custom instructions and coding agent setup ([#150](https://github.com/HerbHall/samverk/issues/150)) ([1f87e3c](https://github.com/HerbHall/samverk/commit/1f87e3cb7a6e3e477b88aafde4c496e012b12468)), closes [#146](https://github.com/HerbHall/samverk/issues/146)
- add dependency graph traversal algorithms ([#105](https://github.com/HerbHall/samverk/issues/105)) ([#109](https://github.com/HerbHall/samverk/issues/109)) ([f1ae67c](https://github.com/HerbHall/samverk/commit/f1ae67cacb52649db023a4efd8598d9e645031ac))
- add GitHub Copilot integration files ([#159](https://github.com/HerbHall/samverk/issues/159)) ([6c7d473](https://github.com/HerbHall/samverk/commit/6c7d473cf2a98a006920a7d2ebb51170c7d4b204))
- add issue CRUD MCP tools ([#111](https://github.com/HerbHall/samverk/issues/111)) ([#121](https://github.com/HerbHall/samverk/issues/121)) ([57e2c23](https://github.com/HerbHall/samverk/commit/57e2c234095dbe94420075accb018e346127a131))
- add MCP repo read operations ([#113](https://github.com/HerbHall/samverk/issues/113)) ([#123](https://github.com/HerbHall/samverk/issues/123)) ([8cdb48e](https://github.com/HerbHall/samverk/commit/8cdb48ecbd194bc160ffdf571c3af5ef463b5a8d))
- add multi-project support with list_projects and set_project tools ([#114](https://github.com/HerbHall/samverk/issues/114)) ([#127](https://github.com/HerbHall/samverk/issues/127)) ([93a5a10](https://github.com/HerbHall/samverk/commit/93a5a105c3bebe0259a512372e8b80a2e76d3697))
- add PR create/merge MCP tools for check-in agent ([#168](https://github.com/HerbHall/samverk/issues/168)) ([f6fd565](https://github.com/HerbHall/samverk/commit/f6fd5655b1275793c12f6c13feb98bbe69f8382d)), closes [#166](https://github.com/HerbHall/samverk/issues/166)
- add provider registry with YAML config and budget enforcement ([#140](https://github.com/HerbHall/samverk/issues/140)) ([1a1d1e3](https://github.com/HerbHall/samverk/commit/1a1d1e3a53b3bb9d58692fd877127fc212932db0))
- add REST API for web dashboard ([#116](https://github.com/HerbHall/samverk/issues/116)) ([#124](https://github.com/HerbHall/samverk/issues/124)) ([1a4d5c3](https://github.com/HerbHall/samverk/commit/1a4d5c34941ca237bb91355c8fb57dfdbd812a35))
- add session recording for MCP tool cost tracking ([#102](https://github.com/HerbHall/samverk/issues/102)) ([#108](https://github.com/HerbHall/samverk/issues/108)) ([6b5c2b0](https://github.com/HerbHall/samverk/commit/6b5c2b03858b4a4ee60051e7cb42daa44e81c6f3))
- add Tier 3 approve/reject with autonomy enforcement ([#112](https://github.com/HerbHall/samverk/issues/112)) ([#122](https://github.com/HerbHall/samverk/issues/122)) ([29c69cd](https://github.com/HerbHall/samverk/commit/29c69cdbd3c0b0bc65dcaf8d82c927ac8b4727ed))
- batch issue ingestion from Claude conversations ([#16](https://github.com/HerbHall/samverk/issues/16)) ([#41](https://github.com/HerbHall/samverk/issues/41)) ([55888eb](https://github.com/HerbHall/samverk/commit/55888eb73fc939264a29110ce27f61407b392fa8))
- Claude and OpenAI provider clients (raw net/http) ([#139](https://github.com/HerbHall/samverk/issues/139)) ([127ca51](https://github.com/HerbHall/samverk/commit/127ca51376327e9961ce40d543577a235fc79b78))
- define IssueTracker interface and implement GitHub adapter ([#34](https://github.com/HerbHall/samverk/issues/34)) ([b621ef5](https://github.com/HerbHall/samverk/commit/b621ef5c0528761e911d50978dfcd05aa63bab63)), closes [#15](https://github.com/HerbHall/samverk/issues/15)
- enable CodeQL security scanning with Copilot Autofix ([#151](https://github.com/HerbHall/samverk/issues/151)) ([b07815e](https://github.com/HerbHall/samverk/commit/b07815e47cce462f1fa9f219b5f4de7282a2ee19)), closes [#147](https://github.com/HerbHall/samverk/issues/147)
- implement agent runtime pool and runner ([#134](https://github.com/HerbHall/samverk/issues/134)) ([#141](https://github.com/HerbHall/samverk/issues/141)) ([7e67dcb](https://github.com/HerbHall/samverk/commit/7e67dcb2b27c8a79b684ab5070c4e55476a8efd4))
- implement autonomy configuration loader ([#37](https://github.com/HerbHall/samverk/issues/37)) ([c424d26](https://github.com/HerbHall/samverk/commit/c424d263d04322693156cb1fdd1c935ebdaf4b7b))
- implement check-in digest prototype (spike [#11](https://github.com/HerbHall/samverk/issues/11)) ([#79](https://github.com/HerbHall/samverk/issues/79)) ([f8a305e](https://github.com/HerbHall/samverk/commit/f8a305ec52eaa9613e2c91d5f12ed41bb8455cc8))
- implement CostSource adapter for SQLite store ([#84](https://github.com/HerbHall/samverk/issues/84)) ([#91](https://github.com/HerbHall/samverk/issues/91)) ([6520f6a](https://github.com/HerbHall/samverk/commit/6520f6a35b327ce90d3fda9c3f7d6911674d3002))
- implement dispatcher core routing loop (Phase 1) ([#62](https://github.com/HerbHall/samverk/issues/62)) ([7db0eaa](https://github.com/HerbHall/samverk/commit/7db0eaa67bf7f796f7d363917ee40d1a7b972dd6)), closes [#58](https://github.com/HerbHall/samverk/issues/58)
- implement frontmatter parser for issue bodies ([#60](https://github.com/HerbHall/samverk/issues/60)) ([d0ef538](https://github.com/HerbHall/samverk/commit/d0ef5380402de166fb0da1221d9e533fb6216dfc)), closes [#57](https://github.com/HerbHall/samverk/issues/57)
- implement Gitea forge adapter ([#56](https://github.com/HerbHall/samverk/issues/56)) ([e8bc25c](https://github.com/HerbHall/samverk/commit/e8bc25cadef6f1f4bf6a3622341351405904d3c2))
- implement HTTP server scaffold with health endpoint ([#81](https://github.com/HerbHall/samverk/issues/81)) ([#88](https://github.com/HerbHall/samverk/issues/88)) ([9ad372f](https://github.com/HerbHall/samverk/commit/9ad372f646ccc7474382208e84c52f586d5e72d7))
- implement MCP handler with get_digest and get_cost_summary tools ([#83](https://github.com/HerbHall/samverk/issues/83)) ([#92](https://github.com/HerbHall/samverk/issues/92)) ([7e36410](https://github.com/HerbHall/samverk/commit/7e364107f396a1d159f406d96488d92b4f95879b))
- implement Ollama provider client ([#54](https://github.com/HerbHall/samverk/issues/54)) ([842402f](https://github.com/HerbHall/samverk/commit/842402f293e1e26fffe3709821eaad0e73c026f1))
- implement profile store with SQLite persistence ([#61](https://github.com/HerbHall/samverk/issues/61)) ([32d2233](https://github.com/HerbHall/samverk/commit/32d22335a5e59d1c5a1262c58c4c5cb2c04d3eac)), closes [#59](https://github.com/HerbHall/samverk/issues/59)
- implement SQLite store layer ([#55](https://github.com/HerbHall/samverk/issues/55)) ([dd886a9](https://github.com/HerbHall/samverk/commit/dd886a9bf92d5a1351b5bfaabcc498bc5c297431))
- initialize Go project structure with Cobra CLI ([#31](https://github.com/HerbHall/samverk/issues/31)) ([af01f49](https://github.com/HerbHall/samverk/commit/af01f491f17515da66ae78f4c2ab9d773d981bfd)), closes [#14](https://github.com/HerbHall/samverk/issues/14)
- LXC deployment scripts and mobile capture idea ([#162](https://github.com/HerbHall/samverk/issues/162)) ([c34f0a2](https://github.com/HerbHall/samverk/commit/c34f0a226521462d8c1c8c9a17b6055db95bb6fa))
- scaffold web dashboard with React + Vite + TypeScript ([#115](https://github.com/HerbHall/samverk/issues/115)) ([#125](https://github.com/HerbHall/samverk/issues/125)) ([00dc0fa](https://github.com/HerbHall/samverk/commit/00dc0fab67a80b7756159b4dc2419dd777f12b60))
- SPA embedding and dashboard pages ([#136](https://github.com/HerbHall/samverk/issues/136), [#137](https://github.com/HerbHall/samverk/issues/137)) ([#143](https://github.com/HerbHall/samverk/issues/143)) ([0284498](https://github.com/HerbHall/samverk/commit/02844984c0ea72f2fe0a587b8b7b06071de0f8b4))
- wire dispatcher CLI command with GitHub watcher ([#117](https://github.com/HerbHall/samverk/issues/117), [#118](https://github.com/HerbHall/samverk/issues/118)) ([#126](https://github.com/HerbHall/samverk/issues/126)) ([22a5bbe](https://github.com/HerbHall/samverk/commit/22a5bbe28a2c96e642c53b4db38a9fd2344ed063))
- wire dispatcher to agent pool for task execution ([#135](https://github.com/HerbHall/samverk/issues/135)) ([#142](https://github.com/HerbHall/samverk/issues/142)) ([f7d1e3c](https://github.com/HerbHall/samverk/commit/f7d1e3cef90e74e4619283837e9858ce96494d62))
- wire forge operations as MCP tools ([#106](https://github.com/HerbHall/samverk/issues/106)) ([2fa8c16](https://github.com/HerbHall/samverk/commit/2fa8c16469ef16b61732834b1244551919025ab7)), closes [#101](https://github.com/HerbHall/samverk/issues/101)

### Bug Fixes

- resolve MD032 lint errors in docs/naming.md ([#35](https://github.com/HerbHall/samverk/issues/35)) ([db3657b](https://github.com/HerbHall/samverk/commit/db3657b25a6300184569e071d5c2b561dd8dd743))
