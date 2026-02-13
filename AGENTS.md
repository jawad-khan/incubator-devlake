<!--
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->
# Apache DevLake - AI Coding Agent Instructions

## Project Overview
Apache DevLake is a dev data platform that ingests data from DevOps tools (GitHub, GitLab, Jira, Jenkins, etc.), transforms it into standardized domain models, and enables metrics/dashboards via Grafana.

## Architecture

### Three-Layer Data Model
1. **Raw Layer** (`_raw_*` tables): JSON data collected from APIs, stored for replay/debugging
2. **Tool Layer** (`_tool_*` tables): Plugin-specific models extracted from raw data
3. **Domain Layer** (standardized tables): Normalized models in [backend/core/models/domainlayer/](backend/core/models/domainlayer/) - CODE, TICKET, CICD, CODEREVIEW, CODEQUALITY, CROSS

### Key Components
- **backend/**: Go server + plugins (main codebase)
- **backend/python/**: Python plugin framework via RPC
- **config-ui/**: React frontend (TypeScript, Vite, Ant Design)
- **grafana/**: Dashboard definitions

## Plugin Development (Go)

### Plugin Structure
Each plugin in `backend/plugins/<name>/` follows this layout:
```
api/         # REST endpoints (connections, scopes, scope-configs)
impl/        # Plugin implementation (implements core interfaces)
models/      # Tool layer models + migrationscripts/
tasks/       # Collectors, Extractors, Converters
e2e/         # Integration tests with CSV fixtures
```

### Required Interfaces
See [backend/plugins/gitlab/impl/impl.go](backend/plugins/gitlab/impl/impl.go) for reference:
- `PluginMeta`: Name, Description, RootPkgPath
- `PluginTask`: SubTaskMetas(), PrepareTaskData()
- `PluginModel`: GetTablesInfo() - **must list all models or CI fails**
- `PluginMigration`: MigrationScripts() for DB schema evolution
- `PluginSource`: Connection(), Scope(), ScopeConfig()

### Subtask Pattern (Collector → Extractor → Converter)
```go
// 1. Register subtask in tasks/register.go via init()
func init() {
    RegisterSubtaskMeta(&CollectIssuesMeta)
}

// 2. Define dependencies for execution order
var CollectIssuesMeta = plugin.SubTaskMeta{
    Name:         "Collect Issues",
    Dependencies: []*plugin.SubTaskMeta{}, // or reference other metas
}
```

### API Collectors
- Use `helper.NewStatefulApiCollector` for incremental collection with time-based bookmarking
- See [backend/plugins/gitlab/tasks/issue_collector.go](backend/plugins/gitlab/tasks/issue_collector.go)

### Migration Scripts
- Located in `models/migrationscripts/`
- Register all scripts in `register.go`'s `All()` function
- Version format: `YYYYMMDD_description.go`

## Build & Development Commands

```bash
# From repo root
make dep              # Install Go + Python dependencies
make build            # Build plugins + server
make dev              # Build + run server
make godev            # Go-only dev (no Python plugins)
make unit-test        # Run all unit tests
make e2e-test         # Run E2E tests

# From backend/
make swag             # Regenerate Swagger docs (required after API changes)
make lint             # Run golangci-lint
```

### Running Locally
```bash
docker-compose -f docker-compose-dev.yml up mysql grafana  # Start deps
make dev                                                     # Run server on :8080
cd config-ui && yarn && yarn start                          # UI on :4000
```

## Testing

### Unit Tests
Place `*_test.go` files alongside source. Use mocks from `backend/mocks/`.

### E2E Tests for Plugins
Use CSV fixtures in `e2e/` directory. See [backend/test/helper/](backend/test/helper/) for the Go test client that can spin up an in-memory DevLake instance.

### Integration Testing
```go
helper.ConnectLocalServer(t, &helper.LocalClientConfig{
    ServerPort:   8080,
    DbURL:        "mysql://merico:merico@127.0.0.1:3306/lake",
    CreateServer: true,
    Plugins:      []plugin.PluginMeta{gitlab.Gitlab{}},
})
```

## Python Plugins
Located in `backend/python/plugins/`. Use Poetry for dependencies. See [backend/python/README.md](backend/python/README.md).

## Code Conventions
- Tool model table names: `_tool_<plugin>_<entity>` (e.g., `_tool_gitlab_issues`)
- Domain model IDs: Use `didgen.NewDomainIdGenerator` for consistent cross-plugin IDs
- All plugins must be independent - no cross-plugin imports
- Apache 2.0 license header required on all source files

## Common Pitfalls
- Forgetting to add models to `GetTablesInfo()` fails `plugins/table_info_test.go`
- Migration scripts must be added to `All()` in `register.go`
- API changes require running `make swag` to update Swagger docs
- Python plugins require `libgit2` for gitextractor functionality

## Asana Plugin Implementation (2025-02-03)

### Overview
Implemented a complete Asana plugin (`backend/plugins/asana/`) to collect projects (boards), sections, and tasks from Asana's REST API and map them to DevLake's ticket/board domain model.

### Architecture Decisions
- **Scope Model**: Asana **Project** = DevLake **Board** (scope)
- **Authentication**: Personal Access Token (PAT) via Bearer token (`Authorization: Bearer <token>`)
- **API Base URL**: `https://app.asana.com/api/1.0/` (default endpoint)
- **Pagination**: Asana uses offset-based pagination via `next_page.offset` in response; implemented sequential fetching with `GetNextPageCustomData`

### Implementation Details

#### Models (`backend/plugins/asana/models/`)
- **Connection** (`connection.go`): `AsanaConn` (Token, RestConnection), `AsanaConnection` (BaseConnection + AsanaConn)
- **Scope** (`project.go`): `AsanaProject` implements `ToolLayerScope`; primary key: `(ConnectionId, Gid)`
- **Scope Config** (`scope_config.go`): `AsanaScopeConfig` embeds `common.ScopeConfig` with `Entities: ["TICKET"]`
- **Tool Layer**:
  - `task.go`: `AsanaTask` (Gid, Name, Notes, Completed, DueOn, ProjectGid, SectionGid, AssigneeGid, CreatorGid, etc.)
  - `section.go`: `AsanaSection` (Gid, Name, ProjectGid)
  - `user.go`: `AsanaUser` (Gid, Name, Email) - optional, for assignee/creator enrichment
- **Migration**: `20250203_add_init_tables.go` creates all tables via `migrationhelper.AutoMigrateTables`

#### API Layer (`backend/plugins/asana/api/`)
- **Connection API** (`connection_api.go`): Test connection via `GET users/me`, CRUD operations, default endpoint handling
- **Scope API** (`scope_api.go`): PutScopes, GetScopeList, GetScope, PatchScope, DeleteScope (uses `:scopeId` path param, not `:projectId`)
- **Scope Config API** (`scope_config_api.go`): Full CRUD for scope configs
- **Blueprint V200** (`blueprint_v200.go`): Maps Asana projects to `ticket.Board` domain entities when scope config includes `DOMAIN_TYPE_TICKET`
- **Remote API** (`remote_api.go`): Proxy endpoint for direct API access
- **Init** (`init.go`): Sets up `DsHelper[AsanaConnection, AsanaProject, AsanaScopeConfig]` with default endpoint

#### Tasks (`backend/plugins/asana/tasks/`)
- **Project**: `project_collector.go` (GET `/projects/{gid}`), `project_extractor.go` (extracts to `_tool_asana_projects`)
- **Section**: `section_collector.go` (GET `/projects/{gid}/sections`), `section_extractor.go` (extracts to `_tool_asana_sections`)
- **Task**: 
  - `task_collector.go`: Collects tasks with offset pagination (limit=100, offset from `next_page.offset`)
  - `task_extractor.go`: Extracts task data including memberships (project/section), assignee, creator, parent
  - `task_convertor.go`: Converts `AsanaTask` → `ticket.Issue` + `ticket.BoardIssue` using `didgen` for domain IDs
- **API Client** (`api_client.go`): Creates `ApiAsyncClient` with Bearer auth, sets default endpoint if missing
- **Task Data** (`task_data.go`): `AsanaOptions` (ConnectionId, ProjectId, ScopeConfigId), `AsanaTaskData`, `CreateRawDataSubTaskArgs` helper

#### Plugin Entry (`backend/plugins/asana/`)
- **Main** (`asana.go`): `PluginEntry impl.Asana` for plugin loading
- **Impl** (`impl/impl.go`): Implements all required interfaces:
  - `PluginMeta`: Name="asana", Description, RootPkgPath
  - `PluginTask`: SubTaskMetas (CollectProject, ExtractProject, CollectSection, ExtractSection, CollectTask, ExtractTask, ConvertTask)
  - `PluginModel`: GetTablesInfo() returns all 6 models
  - `PluginMigration`: MigrationScripts() from migrationscripts.All()
  - `PluginApi`: ApiResources() with connections, scopes, scope-configs, test, proxy routes
  - `PluginSource`: Connection(), Scope(), ScopeConfig()
  - `DataSourcePluginBlueprintV200`: MakeDataSourcePipelinePlanV200()
  - `CloseablePluginTask`: Close() releases ApiClient

#### Config UI (`config-ui/src/plugins/register/asana/`)
- **Config** (`config.tsx`): `AsanaConfig` with:
  - Connection fields: name, endpoint (default: `https://app.asana.com/api/1.0/`), token, proxy, rateLimitPerHour (default: 150)
  - Data scope title: "Projects"
  - Scope config entities: `['TICKET']`
- **Icon** (`assets/icon.svg`): Placeholder SVG icon
- **Registration** (`index.ts`): Exports `AsanaConfig`
- **Plugin Registry** (`config-ui/src/plugins/register/index.ts`): Added `AsanaConfig` to `pluginConfigs` array

#### Testing
- **Table Info Test** (`backend/plugins/table_info_test.go`): Added `asana` import and `checker.FeedIn("asana/models", asana.Asana{}.GetTablesInfo)`
- **E2E Test** (`backend/plugins/asana/e2e/task_test.go`): 
  - Imports raw task CSV (`e2e/raw_tables/_raw_asana_tasks.csv`)
  - Runs `ExtractTaskMeta` subtask
  - Verifies `_tool_asana_tasks` against snapshot (`e2e/snapshot_tables/_tool_asana_tasks.csv`)

### Key Implementation Notes
1. **Scope ID**: Uses `:scopeId` path param (not `:projectId`) to match generic scope helper expectations; scope ID is Asana project GID
2. **Response Parsing**: Asana API wraps responses in `{"data": {...}}` or `{"data": [...], "next_page": {...}}`; collectors unwrap `data` field
3. **Date Parsing**: `due_on` is date-only string (`YYYY-MM-DD`); `parseAsanaDate()` helper converts to `*time.Time`
4. **Task Memberships**: Tasks can belong to multiple projects/sections; extractor uses first section from memberships array
5. **Domain Conversion**: Task status maps: `completed=true` → `ticket.DONE`, `completed=false` → `ticket.TODO`
6. **Default Endpoint**: Set in `api/init.go` constant and applied in `PostConnections` if missing from request

### Files Created
- **Backend**: 25+ Go files across `models/`, `api/`, `tasks/`, `impl/`, `e2e/`
- **Config UI**: 3 TypeScript files (`config.tsx`, `index.ts`, `assets/icon.svg`)
- **Test**: 1 CSV fixture pair (raw + snapshot), 1 E2E test file
- **CI**: Updated `table_info_test.go`

### Next Steps (Optional Enhancements)
- Add user collection/enrichment for assignee/creator names
- Support OAuth 2.0 authentication (currently PAT only)
- Add task comments/subtasks collection
- Implement incremental collection with time-based bookmarking
- Add transformation rules for custom field mappings
