# Changelog

## [0.1.11](https://github.com/HerbHall/samverk/compare/v0.1.10...v0.1.11) (2026-03-09)


### Features

* **#240:** add runner heartbeat to prevent dispatcher timeout restarts ([#353](https://github.com/HerbHall/samverk/issues/353)) ([65c11bd](https://github.com/HerbHall/samverk/commit/65c11bdb05c84a43ca2a2adbd3f8463801131619))
* **#277:** ErrNotSupported for Gitea search_code; MCP returns friendly message ([#352](https://github.com/HerbHall/samverk/issues/352)) ([0c5502d](https://github.com/HerbHall/samverk/commit/0c5502d05d39a0a982a85e66eb73a6778d787f26))
* **#289:** add Metrics page to SPA dashboard ([#334](https://github.com/HerbHall/samverk/issues/334)) ([7dbde43](https://github.com/HerbHall/samverk/commit/7dbde435d6e5bbd78a1b94425aa83e2ded2df3b4))
* **#293:** implement scaling policy engine with configurable thresholds ([#337](https://github.com/HerbHall/samverk/issues/337)) ([1a198fc](https://github.com/HerbHall/samverk/commit/1a198fccf4400a9a771228d6c45c9f172a2fe479))
* **#294:** autoscaler loop connecting policy to pool ([#338](https://github.com/HerbHall/samverk/issues/338)) ([c0440e8](https://github.com/HerbHall/samverk/commit/c0440e8000cd63b89660780e9d4e97c30fdf3c02))
* **#296:** scaling events to dashboard, MCP digest, and durable store ([#340](https://github.com/HerbHall/samverk/issues/340)) ([32e0a2b](https://github.com/HerbHall/samverk/commit/32e0a2bd9fa2606e3a9ea46b6dbec8111b65e589))
* **#299:** task-type duration profiling for smarter routing ([#343](https://github.com/HerbHall/samverk/issues/343)) ([7ad8794](https://github.com/HerbHall/samverk/commit/7ad87945996c5f84b53bbba2e63959ac0dc171e2))
* **#300,#301:** samverk scale CLI, REST API, and MCP scaling tools ([#341](https://github.com/HerbHall/samverk/issues/341)) ([de1a046](https://github.com/HerbHall/samverk/commit/de1a0469ddd77f1b3147189cf45f84e85aa57b4f))
* **#305,#306:** PC agent workspace module and design doc ([#344](https://github.com/HerbHall/samverk/issues/344)) ([71dac4e](https://github.com/HerbHall/samverk/commit/71dac4e0b282a09220c906d0e16d2091cd9f063d))
* **#307,#308:** PC agent forge poller, prompt formatter, and CC launcher ([#345](https://github.com/HerbHall/samverk/issues/345)) ([cf6ff65](https://github.com/HerbHall/samverk/commit/cf6ff650d9e0e2f209a863108bd3eb7365cb197b))
* **#309:** PC agent post-task handler ([#346](https://github.com/HerbHall/samverk/issues/346)) ([1233263](https://github.com/HerbHall/samverk/commit/12332637364157647bfc67cd9a8987073f1302e4))
* **#311:** PC agent single-session runner loop ([#347](https://github.com/HerbHall/samverk/issues/347)) ([b3eaead](https://github.com/HerbHall/samverk/commit/b3eaeade747244f85922243f25336b4e524eaa02)), closes [#311](https://github.com/HerbHall/samverk/issues/311)
* **#312:** PC agent autonomy tier gate ([#348](https://github.com/HerbHall/samverk/issues/348)) ([cfc8ec2](https://github.com/HerbHall/samverk/commit/cfc8ec23eaa37535e8e8adff1c5c1733b70ebe33)), closes [#312](https://github.com/HerbHall/samverk/issues/312)
* **#313:** register PC agent as a worker node with Samverk server ([#351](https://github.com/HerbHall/samverk/issues/351)) ([40785b2](https://github.com/HerbHall/samverk/commit/40785b2bb77128a6273f95dc31df13acfa9f76ef))
* **#328:** migrate dispatcher from log.Printf to structured slog ([#350](https://github.com/HerbHall/samverk/issues/350)) ([ffe4c25](https://github.com/HerbHall/samverk/commit/ffe4c25e6ab18287a7700b11a9d87d3c9abb5ed6))
* **#388:** PR review and merge workflow with tier-based policy ([#391](https://github.com/HerbHall/samverk/issues/391)) ([01922a7](https://github.com/HerbHall/samverk/commit/01922a79701646fe0f907f19748efe9e972de11b))
* **agent:** dynamic pool scaling with Resize and max-worker bound ([#292](https://github.com/HerbHall/samverk/issues/292)) ([#357](https://github.com/HerbHall/samverk/issues/357)) ([bcb9e9b](https://github.com/HerbHall/samverk/commit/bcb9e9b7a7d5ddce5da1f9e7e351b57167745d0e))
* **api:** pressure indicator, metrics history, and healthz pressure ([#288](https://github.com/HerbHall/samverk/issues/288)) ([#354](https://github.com/HerbHall/samverk/issues/354)) ([7bf924a](https://github.com/HerbHall/samverk/commit/7bf924aeceaeb6e8e042c654a7490d7290b944a8))
* **ci:** add Gitea Actions CI and security workflows ([#331](https://github.com/HerbHall/samverk/issues/331)) ([7d99199](https://github.com/HerbHall/samverk/commit/7d991998a2b85853432080966a3f58835e2ee83b))
* **config:** add dual-forge support to server.yaml project config ([#326](https://github.com/HerbHall/samverk/issues/326)) ([b6259bc](https://github.com/HerbHall/samverk/commit/b6259bcd00f1eeec93663aca759429f71256ac11))
* **dispatch:** add --forge and --gitea-url flags for dual-forge dispatching ([#387](https://github.com/HerbHall/samverk/issues/387)) ([22776bb](https://github.com/HerbHall/samverk/commit/22776bb23770c6315a0457b7e3b533f741317b4f)), closes [#274](https://github.com/HerbHall/samverk/issues/274)
* **mcp:** pressure indicator in get_digest metrics section ([#290](https://github.com/HerbHall/samverk/issues/290)) ([#356](https://github.com/HerbHall/samverk/issues/356)) ([3f5d17d](https://github.com/HerbHall/samverk/commit/3f5d17d23fef166b0586e7bfbb6dd72402a16ce5))
* **scripts:** add --forge flag to create-issues.sh for Gitea support ([#327](https://github.com/HerbHall/samverk/issues/327)) ([9191c7f](https://github.com/HerbHall/samverk/commit/9191c7fb6f349e84e178bbdeb29bebe337759534))
* **scripts:** add migrate-issues.py for GitHub-to-Gitea issue migration ([#330](https://github.com/HerbHall/samverk/issues/330)) ([ba7a77e](https://github.com/HerbHall/samverk/commit/ba7a77e2d0b2ad34015dc2a4393eb76e89301ef1))
* **web:** pressure indicator on metrics dashboard ([#289](https://github.com/HerbHall/samverk/issues/289)) ([#355](https://github.com/HerbHall/samverk/issues/355)) ([31b36de](https://github.com/HerbHall/samverk/commit/31b36dedbc30e984135a563024ab2b501fbbe67e))


### Bug Fixes

* **pc-agent:** fix E2E bugs in PowerShell modules ([#310](https://github.com/HerbHall/samverk/issues/310)) ([#362](https://github.com/HerbHall/samverk/issues/362)) ([ea75111](https://github.com/HerbHall/samverk/commit/ea751117ca0247b1d242afdaf73217a90c2dd360))

## [0.1.10](https://github.com/HerbHall/samverk/compare/v0.1.9...v0.1.10) (2026-03-09)


### Features

* add resolve-batch-deps.py; fix create-issues.sh for Windows UTF-8 and milestone ([41a636b](https://github.com/HerbHall/samverk/commit/41a636b6067001511959ccdd53b5928ea32c5eee))
* **gitea:** implement RepoWriter, RepoReader, PullRequestManager + system metrics (B01-B09, W01-W03) ([#319](https://github.com/HerbHall/samverk/issues/319)) ([c4fcd14](https://github.com/HerbHall/samverk/commit/c4fcd144bdd1231d5f18bfa3818cbdecdd289334))


### Bug Fixes

* **#318:** add completion callback from agent pool to dispatcher ([#320](https://github.com/HerbHall/samverk/issues/320)) ([da2671b](https://github.com/HerbHall/samverk/commit/da2671bff737b03609f8e7d67cc970fe264e19a0))
* align status.md phase with Samverk lifecycle naming ([ffbfd9c](https://github.com/HerbHall/samverk/commit/ffbfd9c544e55aeaa96f50875a8e84e34b15dff4))

## [0.1.9](https://github.com/HerbHall/samverk/compare/v0.1.8...v0.1.9) (2026-03-08)


### Bug Fixes

* **#228:** graceful shutdown ([#229](https://github.com/HerbHall/samverk/issues/229)) ([8f36493](https://github.com/HerbHall/samverk/commit/8f364931b2a48efd792163a9a787b228689af06a))
* **#231:** use RELEASE_PLEASE_TOKEN so release PRs trigger CI ([#232](https://github.com/HerbHall/samverk/issues/232)) ([24fd69b](https://github.com/HerbHall/samverk/commit/24fd69bd097a3e8104bb783f525d16ac3c77b6cf))
* **#247:** dispatcher skips issues with terminal status labels and human agent type ([#253](https://github.com/HerbHall/samverk/issues/253)) ([9bfa374](https://github.com/HerbHall/samverk/commit/9bfa374bde1b5c1381fedbe34b6c7c5120011bee))
* dispatcher fixes — failure counter, 404 suppression, Ollama timeout, heartbeat interval ([#249](https://github.com/HerbHall/samverk/issues/249)) ([b1e3635](https://github.com/HerbHall/samverk/commit/b1e3635fa030212af4b7dc02792ff2d7e38442bc))

## [0.1.8](https://github.com/HerbHall/samverk/compare/v0.1.7...v0.1.8) (2026-03-08)


### Features

* claude-cli provider with multi-model routing and GitHub source access ([#223](https://github.com/HerbHall/samverk/issues/223)) ([b36cd37](https://github.com/HerbHall/samverk/commit/b36cd37d402fb5e4a36685da27876b77d560a784))

## [0.1.7](https://github.com/HerbHall/samverk/compare/v0.1.6...v0.1.7) (2026-03-08)


### Features

* claude-cli provider uses Claude Code auth instead of API credits ([#219](https://github.com/HerbHall/samverk/issues/219)) ([e4041fd](https://github.com/HerbHall/samverk/commit/e4041fdd76dca8befd2b5ea5fdb978ff7f396a73)), closes [#218](https://github.com/HerbHall/samverk/issues/218)

## [0.1.6](https://github.com/HerbHall/samverk/compare/v0.1.5...v0.1.6) (2026-03-08)


### Features

* build and embed React SPA in CI workflow ([#212](https://github.com/HerbHall/samverk/issues/212)) ([482f66b](https://github.com/HerbHall/samverk/commit/482f66ba982a2ea3f60759d0d062e9419ee34bf4)), closes [#197](https://github.com/HerbHall/samverk/issues/197)
* PR watcher auto-merges eligible pull requests ([#209](https://github.com/HerbHall/samverk/issues/209)) ([d6e877f](https://github.com/HerbHall/samverk/commit/d6e877f0230c7b27980406456472a9c8b9b896fe)), closes [#204](https://github.com/HerbHall/samverk/issues/204)


### Bug Fixes

* make redeploy works on Windows, stops services before scp ([#214](https://github.com/HerbHall/samverk/issues/214)) ([6910e9b](https://github.com/HerbHall/samverk/commit/6910e9bdec73d4769963d43e8d290845cca575ce)), closes [#210](https://github.com/HerbHall/samverk/issues/210)

## [0.1.5](https://github.com/HerbHall/samverk/compare/v0.1.4...v0.1.5) (2026-03-08)


### Features

* code-gen and test agents open PRs instead of posting comments ([#207](https://github.com/HerbHall/samverk/issues/207)) ([1bb3a51](https://github.com/HerbHall/samverk/commit/1bb3a5164437c6dde8029d67cc42b7c9af842894)), closes [#195](https://github.com/HerbHall/samverk/issues/195)

## [0.1.4](https://github.com/HerbHall/samverk/compare/v0.1.3...v0.1.4) (2026-03-08)


### Bug Fixes

* dispatcher skips PRs and handles issue.assigned events ([#205](https://github.com/HerbHall/samverk/issues/205)) ([dee0595](https://github.com/HerbHall/samverk/commit/dee05954291b1534cc70488bac4cd887b7dda17b))

## [0.1.3](https://github.com/HerbHall/samverk/compare/v0.1.2...v0.1.3) (2026-03-08)


### Features

* specialized system prompts per agent type ([#200](https://github.com/HerbHall/samverk/issues/200)) ([04fa284](https://github.com/HerbHall/samverk/commit/04fa2849fa0d6a257c8966d04a8adc1eb5aa6735))


### Bug Fixes

* resolve markdownlint errors in CHANGELOG.md and remove lint workarounds ([#199](https://github.com/HerbHall/samverk/issues/199)) ([dff575b](https://github.com/HerbHall/samverk/commit/dff575b7f35a87136cba4a38b6105f554db5714e))

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
