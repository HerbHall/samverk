# Changelog

## [0.1.20](https://github.com/HerbHall/samverk/compare/v0.1.19...v0.1.20) (2026-03-17)


### Features

* add MCP work checkout/checkin protocol for supervised agents ([#640](https://github.com/HerbHall/samverk/issues/640)) ([c644f9e](https://github.com/HerbHall/samverk/commit/c644f9e7f717d0bd368380417b6f5145146faa20)), closes [#637](https://github.com/HerbHall/samverk/issues/637)
* add project lifecycle phases and tags ([#635](https://github.com/HerbHall/samverk/issues/635)) ([e508b94](https://github.com/HerbHall/samverk/commit/e508b942fcdf47e211fcfc543885f10cb53c1b3d))

## [0.1.19](https://github.com/HerbHall/samverk/compare/v0.1.18...v0.1.19) (2026-03-17)


### Features

* add explore-before-code planning step to agent workflow ([#614](https://github.com/HerbHall/samverk/issues/614)) ([0b502a9](https://github.com/HerbHall/samverk/commit/0b502a95cdd7aa2481a079d4ea3052e4b118772d))
* read Copilot review feedback before auto-merge ([#608](https://github.com/HerbHall/samverk/issues/608)) ([#610](https://github.com/HerbHall/samverk/issues/610)) ([dafeffc](https://github.com/HerbHall/samverk/commit/dafeffc966ee53eb68e4f8331a1592ecef088f82))


### Bug Fixes

* improve Claude CLI timeout detection and provider failover ([#606](https://github.com/HerbHall/samverk/issues/606)) ([#611](https://github.com/HerbHall/samverk/issues/611)) ([3534230](https://github.com/HerbHall/samverk/commit/3534230e6a9957b1f7600f7da1671cb4d9fa674e))
* restrict Ollama to triage-only and add output validation guard ([#613](https://github.com/HerbHall/samverk/issues/613)) ([e4559fa](https://github.com/HerbHall/samverk/commit/e4559fa2f3bc66e86ec2d9d02835755678e128c8))

## [0.1.18](https://github.com/HerbHall/samverk/compare/v0.1.17...v0.1.18) (2026-03-17)


### Features

* add doc-audit CLI for documentation drift detection ([#467](https://github.com/HerbHall/samverk/issues/467)) ([#596](https://github.com/HerbHall/samverk/issues/596)) ([d759fd4](https://github.com/HerbHall/samverk/commit/d759fd4aa50a5b76409cec7683fe6c0580eea2f1))
* add Gitea Actions deploy workflow with CI gate ([#603](https://github.com/HerbHall/samverk/issues/603)) ([d310235](https://github.com/HerbHall/samverk/commit/d310235fa4e8dba0c7e5b29a9f876c978ef2382f))
* add provider audit CLI with health checks and SQLite persistence ([#444](https://github.com/HerbHall/samverk/issues/444)) ([#597](https://github.com/HerbHall/samverk/issues/597)) ([66a9e3d](https://github.com/HerbHall/samverk/commit/66a9e3d1e83a250a0a1a131d4363955515f825a2))
* enhanced Agents page with per-agent stats and trending ([#590](https://github.com/HerbHall/samverk/issues/590)) ([#595](https://github.com/HerbHall/samverk/issues/595)) ([3413e6c](https://github.com/HerbHall/samverk/commit/3413e6cc5f20507370cc5e518af873ade59d5a9b))


### Bug Fixes

* remove OAuth endpoints and BearerAuth from MCP handler ([#601](https://github.com/HerbHall/samverk/issues/601)) ([675ac75](https://github.com/HerbHall/samverk/commit/675ac7527dd2534fccfec44f9a572e2d5f0b8313)), closes [#600](https://github.com/HerbHall/samverk/issues/600)
* remove OAuth, add MCP-only listener, fix Host header rejection ([#604](https://github.com/HerbHall/samverk/issues/604)) ([d896936](https://github.com/HerbHall/samverk/commit/d8969365e2db609fcaa9aab2cfa991b5c5a4d0b3)), closes [#600](https://github.com/HerbHall/samverk/issues/600)

## [0.1.17](https://github.com/HerbHall/samverk/compare/v0.1.16...v0.1.17) (2026-03-16)


### Features

* add Synapset proxy and native React dashboard page ([#587](https://github.com/HerbHall/samverk/issues/587)) ([#589](https://github.com/HerbHall/samverk/issues/589)) ([5bbb246](https://github.com/HerbHall/samverk/commit/5bbb246531ca43f23cc2e52ec8995509f89b1760))
* embed DevKit dashboard in Samverk SPA ([#564](https://github.com/HerbHall/samverk/issues/564)) ([#584](https://github.com/HerbHall/samverk/issues/584)) ([c84be39](https://github.com/HerbHall/samverk/commit/c84be398beef44dbef150a98f5de63cb6b615c83))


### Bug Fixes

* remove duplicate Synapset/DevKit links from header ([#592](https://github.com/HerbHall/samverk/issues/592)) ([4efbeb4](https://github.com/HerbHall/samverk/commit/4efbeb4bee6c2d1dcc961a81e2d2da73bb956e7c))
* wider sparklines on all metrics, clickable issue links everywhere ([#591](https://github.com/HerbHall/samverk/issues/591)) ([7d314cd](https://github.com/HerbHall/samverk/commit/7d314cd310a10e831ad83de7b6e3c4ee782a5c0c))

## [0.1.16](https://github.com/HerbHall/samverk/compare/v0.1.15...v0.1.16) (2026-03-16)


### Features

* **#434:** cross-project dependency graph and coordination protocol ([#454](https://github.com/HerbHall/samverk/issues/454)) ([bf40b51](https://github.com/HerbHall/samverk/commit/bf40b51a73b68cf9ca032e85352320b2e457ba02))
* **#436:** integrate Synapset semantic memory into agent runtime ([#449](https://github.com/HerbHall/samverk/issues/449)) ([22a943b](https://github.com/HerbHall/samverk/commit/22a943b400f05e6ce0bce26caaec46bd4b6f6096))
* **#439:** add automated deploy workflow for CT 202 ([#447](https://github.com/HerbHall/samverk/issues/447)) ([7721c81](https://github.com/HerbHall/samverk/commit/7721c81dfd80a09ff210d859452bfd02c8f11808)), closes [#439](https://github.com/HerbHall/samverk/issues/439)
* **#443:** Agents dashboard page with provider cards and account links ([#453](https://github.com/HerbHall/samverk/issues/453)) ([53f948a](https://github.com/HerbHall/samverk/commit/53f948a6d1e421249eef444e87a7f3a876eeba5d))
* add AI log analyst with Ollama-powered summarization ([#551](https://github.com/HerbHall/samverk/issues/551)) ([0033cea](https://github.com/HerbHall/samverk/commit/0033cea357118834ff11e70856be4faa46626fb5))
* add cloudflared tunnel health watchdog ([#478](https://github.com/HerbHall/samverk/issues/478)) ([#571](https://github.com/HerbHall/samverk/issues/571)) ([5b145bd](https://github.com/HerbHall/samverk/commit/5b145bdc416f270c63a03cd340b63b2e873944a2))
* add dark mode toggle to dashboard ([#563](https://github.com/HerbHall/samverk/issues/563)) ([#565](https://github.com/HerbHall/samverk/issues/565)) ([056ce27](https://github.com/HerbHall/samverk/commit/056ce27c9ff5736457dbf9d4f60e2c8280e789d2))
* add dashboard log viewer page ([#510](https://github.com/HerbHall/samverk/issues/510)) ([#550](https://github.com/HerbHall/samverk/issues/550)) ([f45e443](https://github.com/HerbHall/samverk/commit/f45e4439e8b90868961d67cdaa2a35e2d45236f8))
* add host metrics collector with procfs support ([#547](https://github.com/HerbHall/samverk/issues/547)) ([dd895f4](https://github.com/HerbHall/samverk/commit/dd895f4d291c00c4ac7b325b0d111404ed2866e0))
* add host resources section to Metrics page ([#508](https://github.com/HerbHall/samverk/issues/508)) ([#549](https://github.com/HerbHall/samverk/issues/549)) ([51bb4b0](https://github.com/HerbHall/samverk/commit/51bb4b0507c2ad436bedbb30d146555fb81d8392))
* add My Queue dashboard page for human-required issues ([#469](https://github.com/HerbHall/samverk/issues/469)) ([0c76c30](https://github.com/HerbHall/samverk/commit/0c76c301408507d49400939c03c4fdaf64cdb62c))
* add OAuth 2.1 discovery endpoints for Claude mobile MCP ([#479](https://github.com/HerbHall/samverk/issues/479)) ([#570](https://github.com/HerbHall/samverk/issues/570)) ([37e3c17](https://github.com/HerbHall/samverk/commit/37e3c17756c89f1eadc2e95e88bc7f8277606fe3))
* add post-completion quality gate for agent output ([#496](https://github.com/HerbHall/samverk/issues/496)) ([#575](https://github.com/HerbHall/samverk/issues/575)) ([6455fd5](https://github.com/HerbHall/samverk/commit/6455fd50877540eb12457431be2d8036a6d4bee3))
* add pre-posting validation gate for agent output ([#518](https://github.com/HerbHall/samverk/issues/518)) ([#537](https://github.com/HerbHall/samverk/issues/537)) ([8f99410](https://github.com/HerbHall/samverk/commit/8f9941039240bbdc6103938d4b28a74ec63b0bd7))
* add provider health monitor with proactive checks ([#473](https://github.com/HerbHall/samverk/issues/473), [#494](https://github.com/HerbHall/samverk/issues/494)) ([#579](https://github.com/HerbHall/samverk/issues/579)) ([6646a15](https://github.com/HerbHall/samverk/commit/6646a15d781640a0d43a4325654eff74b890ffac))
* add SQLite log tee with query API and auto-pruning ([#548](https://github.com/HerbHall/samverk/issues/548)) ([a7e9157](https://github.com/HerbHall/samverk/commit/a7e9157b38b4f18e4a0c071acef83442a7586baa))
* add structured logging to all provider implementations ([#526](https://github.com/HerbHall/samverk/issues/526)) ([f13e5d5](https://github.com/HerbHall/samverk/commit/f13e5d556381bc4104868546b8a725ab2568030c))
* add Wake-on-LAN support and model deploy script ([#495](https://github.com/HerbHall/samverk/issues/495), [#497](https://github.com/HerbHall/samverk/issues/497)) ([#580](https://github.com/HerbHall/samverk/issues/580)) ([76a5a0d](https://github.com/HerbHall/samverk/commit/76a5a0d00af1a3584d409981adf988f3babac48e))
* auto-inject frontmatter for issues classified by heuristic ([#475](https://github.com/HerbHall/samverk/issues/475)) ([#574](https://github.com/HerbHall/samverk/issues/574)) ([44f42cf](https://github.com/HerbHall/samverk/commit/44f42cf06a6bf6e3e4b6415fc1a0c08997e48445))
* **dashboard:** add metric tooltips and capacity section ([#437](https://github.com/HerbHall/samverk/issues/437)) ([90c6f0a](https://github.com/HerbHall/samverk/commit/90c6f0a58cf958243581ad072ed55af4e662531e))
* equip agents with DevKit rules, MCP, and Synapset tools ([#531](https://github.com/HerbHall/samverk/issues/531)) ([ab488c8](https://github.com/HerbHall/samverk/commit/ab488c8939eec636ef1f8d41575161dd5cf83042))
* equip agents with DevKit rules, project type detection, and git safety ([#521](https://github.com/HerbHall/samverk/issues/521)) ([#539](https://github.com/HerbHall/samverk/issues/539)) ([889befe](https://github.com/HerbHall/samverk/commit/889befeaa0f8daa42af205be7353f91f1c75ca04))
* extend metrics trending window from 30m to 24h ([#499](https://github.com/HerbHall/samverk/issues/499)) ([12cf609](https://github.com/HerbHall/samverk/commit/12cf609905ea4cd0f357bdf6d884efdeaead9575))
* implement autonomy tier enforcement in agent runner ([#522](https://github.com/HerbHall/samverk/issues/522)) ([627e784](https://github.com/HerbHall/samverk/commit/627e784b5dfa2544a0e185d498dc1dab5da87463)), closes [#515](https://github.com/HerbHall/samverk/issues/515)
* implement intelligent failure response engine ([#525](https://github.com/HerbHall/samverk/issues/525)) ([eeb8363](https://github.com/HerbHall/samverk/commit/eeb8363dd63fdefb05751167ae35dc7f7081e8d4))
* implement isolated agent workspaces via git worktrees ([#517](https://github.com/HerbHall/samverk/issues/517)) ([#529](https://github.com/HerbHall/samverk/issues/529)) ([6920536](https://github.com/HerbHall/samverk/commit/69205367c04554d5f0594e853aaa6b680297fbeb))
* session-end documentation gate in agent runner ([#466](https://github.com/HerbHall/samverk/issues/466)) ([#581](https://github.com/HerbHall/samverk/issues/581)) ([88f5025](https://github.com/HerbHall/samverk/commit/88f5025246190e7e9049eeefde2395b3efca409e))
* unified dashboard header with version display ([#569](https://github.com/HerbHall/samverk/issues/569)) ([#583](https://github.com/HerbHall/samverk/issues/583)) ([65d0a0d](https://github.com/HerbHall/samverk/commit/65d0a0da849edd0422dc4f3632bb50213624e699))
* wire SetRepoDir through pool to runner with FetchLatest ([#536](https://github.com/HerbHall/samverk/issues/536)) ([db6afec](https://github.com/HerbHall/samverk/commit/db6afec238ffdd63ee12df393fec6e17c2a260bf))


### Bug Fixes

* add --allowedTools and --max-turns to claudecli provider ([#502](https://github.com/HerbHall/samverk/issues/502)) ([1dc13b4](https://github.com/HerbHall/samverk/commit/1dc13b4d5c637fc1f94c14d60aa397c1592653a8))
* add --no-session-persistence to claudecli provider ([#512](https://github.com/HerbHall/samverk/issues/512)) ([0c9fd6e](https://github.com/HerbHall/samverk/commit/0c9fd6e4ea79db5ae33c19270dc79c735ad5db56))
* add Accept header to Synapset client for JSON content negotiation ([#546](https://github.com/HerbHall/samverk/issues/546)) ([94b0df8](https://github.com/HerbHall/samverk/commit/94b0df8ea937851bb82329152aee2bf8b3061753)), closes [#544](https://github.com/HerbHall/samverk/issues/544)
* add nolint for gosec G117 on OAuth access_token field ([#572](https://github.com/HerbHall/samverk/issues/572)) ([bc602bc](https://github.com/HerbHall/samverk/commit/bc602bc1c8bc22da6ee01e43588c6aaac4a81e21))
* add SQLite busy_timeout to prevent SQLITE_BUSY contention ([#545](https://github.com/HerbHall/samverk/issues/545)) ([53099c5](https://github.com/HerbHall/samverk/commit/53099c5ed665ab7e9ae4aad981e05a780e67d486)), closes [#538](https://github.com/HerbHall/samverk/issues/538)
* classify "not logged in" as auth failure and fix Synapset pool parameter ([#534](https://github.com/HerbHall/samverk/issues/534)) ([ac35e77](https://github.com/HerbHall/samverk/commit/ac35e77962ecb85a55318e7394283d2ada3e77c3))
* deploy pipeline waits for idle dispatcher and always rebuilds SPA ([#471](https://github.com/HerbHall/samverk/issues/471)) ([ff046b0](https://github.com/HerbHall/samverk/commit/ff046b0ac4fba68669f3861e7a5cef27c587e2aa))
* **deploy:** add --providers-config to serve service file ([66a0797](https://github.com/HerbHall/samverk/commit/66a0797c6e158766c2ca444f0099731dd142d276))
* guard against posting empty comments ([#474](https://github.com/HerbHall/samverk/issues/474)) ([#573](https://github.com/HerbHall/samverk/issues/573)) ([cd0cb7d](https://github.com/HerbHall/samverk/commit/cd0cb7d5c5bb0ca1bbfe26f2312d1305f48987c0))
* immediate dispatch when worker becomes idle ([#485](https://github.com/HerbHall/samverk/issues/485)) ([#567](https://github.com/HerbHall/samverk/issues/567)) ([7e23cd0](https://github.com/HerbHall/samverk/commit/7e23cd04bd3ba829470baa8e5475405496947c13))
* make issue assignment best-effort for Gitea compatibility ([#562](https://github.com/HerbHall/samverk/issues/562)) ([ec325d0](https://github.com/HerbHall/samverk/commit/ec325d00d147ee29523dbfc87a733b8dc5c33612))
* provider-down failures no longer escalate issues to needs-human ([#482](https://github.com/HerbHall/samverk/issues/482)) ([e541d2d](https://github.com/HerbHall/samverk/commit/e541d2da172c0182c59a3d2824d697bc28bab445)), closes [#472](https://github.com/HerbHall/samverk/issues/472)
* reduce CLI hang detection to 60s and add spawn stagger ([#491](https://github.com/HerbHall/samverk/issues/491)) ([#568](https://github.com/HerbHall/samverk/issues/568)) ([2c1608a](https://github.com/HerbHall/samverk/commit/2c1608a4817c4bc4f676e5c1e6f81b097017e0fa))
* restart watcher on failure with backoff and force-push agent branches ([#582](https://github.com/HerbHall/samverk/issues/582)) ([dbd0c53](https://github.com/HerbHall/samverk/commit/dbd0c536643aefc936ff7eeecc6ab5740bcf143a))
* return 405 for non-POST requests to /mcp instead of SPA HTML ([#477](https://github.com/HerbHall/samverk/issues/477)) ([fbdc96a](https://github.com/HerbHall/samverk/commit/fbdc96a3c8593e4c380686c7530372e0b5c65efc))
* set providers healthy in dashboard and document metric counter lifecycle ([#566](https://github.com/HerbHall/samverk/issues/566)) ([2272b83](https://github.com/HerbHall/samverk/commit/2272b83ad86deb428fe8f37167f12b587708af45))
* set SQLite pragmas via DSN for all pooled connections ([#561](https://github.com/HerbHall/samverk/issues/561)) ([db8908f](https://github.com/HerbHall/samverk/commit/db8908fa885e48615bb3ee37c6e6dde10ffca75a))
* skip worktree validation when go tool is not installed ([#542](https://github.com/HerbHall/samverk/issues/542)) ([8fc4056](https://github.com/HerbHall/samverk/commit/8fc4056e28988e65efcbfc3c7d74800fb4a043c0))
* **synapset:** handle non-JSON responses and add post-init stabilization delay ([#452](https://github.com/HerbHall/samverk/issues/452)) ([2f6d6f9](https://github.com/HerbHall/samverk/commit/2f6d6f9596e1822070ee553ced3d4e2bcd62ca9f)), closes [#450](https://github.com/HerbHall/samverk/issues/450)

## [0.1.15](https://github.com/HerbHall/samverk/compare/v0.1.14...v0.1.15) (2026-03-14)


### Bug Fixes

* **dashboard:** rebuild SPA with auth token support ([5f55d53](https://github.com/HerbHall/samverk/commit/5f55d534147ed1fbacd6cc9c787d7d096dea5b92))

## [0.1.14](https://github.com/HerbHall/samverk/compare/v0.1.13...v0.1.14) (2026-03-14)


### Features

* **#186:** add samverk status CLI command ([#427](https://github.com/HerbHall/samverk/issues/427)) ([7ff6d71](https://github.com/HerbHall/samverk/commit/7ff6d71a9d09fbf0a01b190bc303a5dbe521e142))
* **#245:** pre-flight issue decomposition for oversized tasks ([#430](https://github.com/HerbHall/samverk/issues/430)) ([182c2f8](https://github.com/HerbHall/samverk/commit/182c2f82bca8e8be099bfb93882c8c9e87a8cab6))
* **#246:** timeout calibration feedback loop with historical p90 auto-tuning ([#429](https://github.com/HerbHall/samverk/issues/429)) ([8c3a5d3](https://github.com/HerbHall/samverk/commit/8c3a5d363657a3b6ce58caf77c9377f1ca2c665e)), closes [#246](https://github.com/HerbHall/samverk/issues/246)
* **#323:** automated failure analysis loop — classification, persistence, circuit breakers ([#431](https://github.com/HerbHall/samverk/issues/431)) ([e110215](https://github.com/HerbHall/samverk/commit/e110215986fb0942ebaa379d7a65260d5c3fe7cd)), closes [#323](https://github.com/HerbHall/samverk/issues/323)
* **#359:** promote FormatDuration to public pkg/models helper ([#424](https://github.com/HerbHall/samverk/issues/424)) ([869033b](https://github.com/HerbHall/samverk/commit/869033b791fc7588abb7c9a254abd830c07eec94)), closes [#359](https://github.com/HerbHall/samverk/issues/359)


### Bug Fixes

* **#226:** restore systemd hardening with .claude ReadWritePaths ([#425](https://github.com/HerbHall/samverk/issues/425)) ([2a85469](https://github.com/HerbHall/samverk/commit/2a85469dbaf2c1024501c09cd842bbf769d364ef)), closes [#226](https://github.com/HerbHall/samverk/issues/226)

## [0.1.13](https://github.com/HerbHall/samverk/compare/v0.1.12...v0.1.13) (2026-03-14)


### Features

* **#243:** streaming progress detection with heartbeat reset on active output ([#405](https://github.com/HerbHall/samverk/issues/405)) ([2d10b64](https://github.com/HerbHall/samverk/commit/2d10b6406afbc800800c95aef0ab611ebc76e43e)), closes [#243](https://github.com/HerbHall/samverk/issues/243)
* **#244:** session checkpoint and resume — carry partial work across retries ([#406](https://github.com/HerbHall/samverk/issues/406)) ([9ad7b95](https://github.com/HerbHall/samverk/commit/9ad7b95e536662f9f74c6b059ba9863ed3f05c62)), closes [#244](https://github.com/HerbHall/samverk/issues/244)
* **#412:** cross-model QC routing via dedicated provider chain ([#416](https://github.com/HerbHall/samverk/issues/416)) ([b3b68c2](https://github.com/HerbHall/samverk/commit/b3b68c22e1dc1e2cbc0abeeac86f1b6b56c86c49)), closes [#412](https://github.com/HerbHall/samverk/issues/412)
* **#413:** PROGRESS comment protocol for periodic mid-task state ([#418](https://github.com/HerbHall/samverk/issues/418)) ([1238233](https://github.com/HerbHall/samverk/commit/1238233950004712007feeda43103dfc6a85dd0d))
* **#414:** per-issue token aggregation with outlier detection ([#417](https://github.com/HerbHall/samverk/issues/417)) ([b68fa73](https://github.com/HerbHall/samverk/commit/b68fa7399f413cf84e7af0478dfcdeaa632903ac)), closes [#414](https://github.com/HerbHall/samverk/issues/414)
* **auth:** add scoped worker identity to KeyStore ([#410](https://github.com/HerbHall/samverk/issues/410)) ([#422](https://github.com/HerbHall/samverk/issues/422)) ([af28a78](https://github.com/HerbHall/samverk/commit/af28a78f9b268ebb2675bfec525e8a2708f8aa0c))
* **dashboard:** inject auth token into SPA for API access ([#409](https://github.com/HerbHall/samverk/issues/409)) ([#421](https://github.com/HerbHall/samverk/issues/421)) ([fa5e386](https://github.com/HerbHall/samverk/commit/fa5e386fda22b1f6cca64840c63289fdf94debe4))
* **dispatcher:** dynamic per-issue timeout based on complexity ([#402](https://github.com/HerbHall/samverk/issues/402)) ([7b880c8](https://github.com/HerbHall/samverk/commit/7b880c8fffb03180188c48fc0474b9316f3c0f09))


### Bug Fixes

* **#399:** bridge cross-process metrics gap between dispatch and serve ([#423](https://github.com/HerbHall/samverk/issues/423)) ([746b081](https://github.com/HerbHall/samverk/commit/746b0819744a2633a4fab93f352dcadea0c2d4f2)), closes [#399](https://github.com/HerbHall/samverk/issues/399)
* Copilot [#402](https://github.com/HerbHall/samverk/issues/402) followup + multi-agent research docs ([#415](https://github.com/HerbHall/samverk/issues/415)) ([ea1c728](https://github.com/HerbHall/samverk/commit/ea1c728b63adad654e2112851c47717a9f670f4f))
* **forge:** query check runs API for GitHub Actions CI status ([#401](https://github.com/HerbHall/samverk/issues/401)) ([#403](https://github.com/HerbHall/samverk/issues/403)) ([8b79f3a](https://github.com/HerbHall/samverk/commit/8b79f3a0a87814ee9c33b1e14f3818123f03e797))
* **server:** protect API routes with BearerAuth middleware ([#407](https://github.com/HerbHall/samverk/issues/407), [#408](https://github.com/HerbHall/samverk/issues/408)) ([#420](https://github.com/HerbHall/samverk/issues/420)) ([7af1a7c](https://github.com/HerbHall/samverk/commit/7af1a7c07f91d2862648e53fca6c14ec99a7610d))

## [0.1.12](https://github.com/HerbHall/samverk/compare/v0.1.11...v0.1.12) (2026-03-09)


### Features

* **#389:** Gitea AI code review with Claude/Ollama ([#393](https://github.com/HerbHall/samverk/issues/393)) ([ed50f4b](https://github.com/HerbHall/samverk/commit/ed50f4b3ef61344c152c25e79fb63ae2e57e462e))
* **logging:** migrate from slog to zap with dual-mode output ([#397](https://github.com/HerbHall/samverk/issues/397)) ([1a7d4dc](https://github.com/HerbHall/samverk/commit/1a7d4dc4774ea503b30e2fb89a3277bd7d9e27e8)), closes [#390](https://github.com/HerbHall/samverk/issues/390)


### Bug Fixes

* **ci:** ensure curl is installed before trivy install on Gitea runner ([aadc45f](https://github.com/HerbHall/samverk/commit/aadc45f3acdd625f72e8079713ea073e811fe9cf))
* **ci:** inline SPA build in Gitea CI ([#400](https://github.com/HerbHall/samverk/issues/400)) ([3350fa9](https://github.com/HerbHall/samverk/commit/3350fa98b46303e804d43e6133867fc89302dfc7))
* **ci:** skip node_modules in trivy scan and fix disk space on runner ([705c904](https://github.com/HerbHall/samverk/commit/705c9049290834c14b1dac78479882b3fb22213c))
* **forge:** add pagination to label cache, issue listing, and comments ([#398](https://github.com/HerbHall/samverk/issues/398)) ([8b998cb](https://github.com/HerbHall/samverk/commit/8b998cb4a62ed3b0bd6195bb9262305c94563318)), closes [#394](https://github.com/HerbHall/samverk/issues/394)

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
