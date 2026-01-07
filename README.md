# Silky

**Silky** is a declarative, YAML-configured API crawler designed for complex and dynamic API data extraction. It allows developers to describe multi-step API interactions with support for nested operations, data transformations, and context-based processing.

The core functionality of Silky revolves around three main step types:

* `request`: to perform API calls,
* `forEach`: to iterate over arrays extracted from context data,
* `forValues`: to iterate over literal values defined in the configuration.

Each step operates in its own **context**, allowing for precise manipulation and isolation of data. Contexts are organized in a hierarchical structure, with each `forEach` or `forValues` step creating new child contexts. This enables fine-grained control of nested operations and data scoping. After execution, results can be merged into parent or ancestor contexts using declarative **merge rules**.

Silky also supports:

* Response transformation via `jq` expressions
* Request templating with Go templates
* Global and request-level authentication and headers
* Multiple authentication mechanisms: OAuth2 (password and client_credentials flows), Bearer tokens, Basic auth, Cookie-based auth, JWT auth, and fully customizable authentication
* Streaming of top-level entities when operating on array-based root contexts
* Parallel execution of `forEach` iterations with configurable concurrency and rate limiting

To simplify development, Silky includes a **configuration builder CLI tool**, written in Go, that enables real-time execution and inspection of the configuration. This tool helps developers debug and refine their manifests by visualizing intermediate steps.

The library comes with a [developer IDE](cmd/ide/) which helps in building, debugging and analyze crawl configuration.

![ide](assets/ide_showcase.gif)


---

## Features

* Declarative configuration using YAML
* Supports nested data traversal and merging
* Powerful context hierarchy system for scoped operations
* Built-in support for `jq` and Go templates
* Multiple authentication types (OAuth2, Basic, Bearer, Cookie, JWT, Custom)
* Parallel execution support for forEach steps with rate limiting
* Config builder with live evaluation and inspection
* Streaming support for root-level arrays

---

## Installation & Development Tools

Silky provides two development tools to help build, test, and debug configurations:

### Terminal IDE (TUI)

The terminal-based IDE provides an interactive environment for developing Silky configurations with real-time execution feedback.

**Installation:**
```bash
# Build the IDE
cd cmd/ide && go build -o silky-ide

# Or run directly
cd cmd/ide && go run .
```

**Features:**
- File watcher with auto-restart on configuration changes
- Step-by-step execution visualization
- Context inspection at each step
- Export execution tree to `/out` folder for debugging
- Keyboard shortcuts for navigation and control

**Usage:**
```bash
# Run the IDE and select a configuration file
./silky-ide

# Or specify a file directly
./silky-ide path/to/config.silky.yaml
```

**Keyboard Shortcuts:**
| Key | Action |
|-----|--------|
| `Enter` | Select step / Expand details |
| `j`/`k` or `Up`/`Down` | Navigate steps |
| `c` | View context map |
| `v` | Set runtime variables (JSON) |
| `r` | Restart execution |
| `s` | Stop execution |
| `d` | Dump execution tree to /out |
| `q` | Quit |
| `?` | Show help |

### VS Code Extension

The VS Code extension provides IDE integration with syntax highlighting, validation, snippets, and execution capabilities.

**Installation:**

1. **From GitHub Release (Recommended):**
   - Go to [Releases](https://github.com/noi-techpark/go-silky/releases)
   - Download the latest `silky-vscode-*.vsix` file
   - Install via command line:
     ```bash
     code --install-extension silky-vscode-0.1.0.vsix
     ```
   - Or install via VS Code UI:
     1. Open VS Code
     2. Go to Extensions (Ctrl+Shift+X)
     3. Click the `...` menu at the top of the Extensions panel
     4. Select "Install from VSIX..."
     5. Browse to the downloaded `.vsix` file

2. **Build from Source:**
   ```bash
   # Build the extension
   cd vscode-extension
   npm install
   npm run package

   # Install in VS Code
   code --install-extension silky-vscode-*.vsix
   ```

3. **For Development:**
   ```bash
   cd vscode-extension
   npm install
   npm run compile
   # Press F5 in VS Code to launch Extension Development Host
   ```

**Prerequisites:**
- VS Code 1.80.0 or higher
- [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml) by Red Hat (will be prompted to install)

**Build Prerequisites (only if building from source):**
- Go 1.21+ (for building binaries)
- Node.js 18+ (for building extension)

**Features:**
- JSON Schema validation for `.silky.yaml` files
- 37 code snippets with `silky-` prefix
- Execution with step-by-step timeline visualization
- Context and result inspection
- Integrated profiler view

**Usage:**
1. Create a file with `.silky.yaml` extension
2. Use snippets: type `silky-` and select from autocomplete
3. Click the play button in the editor title bar to run
4. View execution steps in the Silky sidebar panel
5. Click on steps to inspect context and results

**Configuration:**
```json
{
  "silky.autoValidate": true,
  "silky.autoRun": false,
  "silky.maxOutputSize": 10000,
  "silky.collapseSteps": false
}
```

---

## Context System

Silky's context system is the foundation of its data processing capabilities. Understanding how contexts work is essential for building effective crawl configurations.

### Context Hierarchy

When the crawler starts, it initializes a **root context** containing the initial data structure (either an empty array `[]` or empty object `{}`). As steps execute:

1. **ForEach steps** create a new child context for each iteration, extracting items from a path in the current context
2. **ForValues steps** create an overlay context for each literal value, preserving access to parent context variables
3. **Request steps** create a working context with the response data; nested steps operate on the response
4. Each context has a unique key and maintains a reference to its parent context
5. All ancestor contexts remain accessible via the context map

### Canonical vs Working Contexts

Silky distinguishes between two types of contexts:

* **Canonical contexts**: Named contexts that persist throughout execution (e.g., "root", contexts created by `forEach` with `as`, contexts created by `forValues` with `as`). These are the targets for `mergeWithContext` operations.
* **Working contexts**: Temporary contexts created by request steps to hold response data. When a request executes within a canonical context (like "root"), it creates a working context with a `_response_` prefix internally, ensuring the canonical context remains available for merge operations.

This architecture ensures that `mergeWithContext: {name: root}` always merges into the actual root context, not a cloned copy.

### Context Variables

Within templates and `jq` expressions, you can reference:

* **Named contexts**: Access any context by its `as` name (e.g., `.language`, `.location`, `.data`) in Go templates
* **Special variable `$res`**: In merge rules, refers to the result being merged
* **Special variable `$ctx`**: In transform and merge rules, provides access to the full context map as an object
* **Context map access**: Use `$ctx.contextName` to access any named context from jq expressions

### Understanding Parent Context in Merge Operations

It's important to understand what "parent" means for merge operations:

**In forEach steps:** The parent is the context the forEach is executing within. For example:
```yaml
rootContext: {items: [{id: 1}, {id: 2}]}
steps:
  - type: forEach
    path: .items
    as: item
    steps:
      - type: request
        mergeWithParentOn: .result = $res  # Parent is ROOT, not .items!
```
Even though forEach operates on `.items` path, the parent context for merge is the **root context**, not the `.items` array.

**In request steps with `as`:** The parent is still the context the request is executing within:
```yaml
forEach as: language
  request as: data
    mergeWithParentOn: .[$ctx.language.value] = $res  # Parent is "language" context
```

### Merge Strategies

After a step executes, its result can be merged back into a context using several strategies:

* **`mergeOn`**: Merge with current context using a jq expression (e.g., `.items = $res`)
* **`mergeWithParentOn`**: Merge with immediate parent context using a jq expression (see above for what "parent" means)
* **`mergeWithContext`**: Merge with a named ancestor context (e.g., `{name: "facility", rule: ".details = $res"}`)
* **`noopMerge: true`**: Skip merging entirely (useful when nested steps handle their own merging)
* **Default**: If no merge option is specified, arrays are appended and objects are shallow-merged

These options are mutually exclusive - only one can be specified per step.

---

## Runtime Variables

Silky supports runtime variables that can be injected into your configuration at execution time. This allows you to:

- Pass API keys, tokens, or secrets without hardcoding them
- Configure environment-specific values (production, staging, development)
- Share configurations across different contexts

### Variable Injection

Variables are passed as a `map[string]any` to the `Run()` method and are accessible in:

1. **URL templates**: Use `{{ .variableName }}`
2. **Headers**: Use `{{ .variableName }}`
3. **Request body**: Use `{{ .variableName }}` (recursively in nested objects)
4. **Merge rules (JQ expressions)**: Use `$ctx.variableName`

Variables are injected at the root level of the template context with highest priority, meaning they override any context values with the same name.

### CLI Usage

```bash
# Pass variables via JSON flag
silky -config my-config.silky.yaml -vars '{"apiKey":"abc123","env":"prod"}'

# Complex variables
silky -config config.yaml -vars '{"auth":{"user":"admin","pass":"secret"},"limit":100}'
```

### Example Configuration

```yaml
rootContext: {}
steps:
  - type: request
    name: Fetch Data
    request:
      url: "https://api.example.com/{{ .env }}/data"
      method: POST
      headers:
        Authorization: "Bearer {{ .apiKey }}"
        Content-Type: application/json
      body:
        query: "{{ .searchTerm }}"
        filters:
          startDate: "{{ .startFrom }}"
          endDate: "{{ .endTo }}"
    resultTransformer: |
      .items | map(select(.category == $ctx.category))
```

**Run with:**
```bash
silky -config example.yaml -vars '{"env":"production","apiKey":"secret123","searchTerm":"test","startFrom":"2024-01-01","endTo":"2024-12-31","category":"active"}'
```

### Terminal IDE Usage

Press `v` to open the variables input modal. Enter JSON and press Save. Variables persist across restarts until modified.

### VS Code Extension Usage

The VS Code extension provides a dedicated **Variables** panel in the Silky sidebar (similar to the Watch panel in debuggers):

1. **Variables Panel** - Located under "Execution Steps" in the Silky sidebar
   - Click `+` to add a new variable
   - Click the edit icon on a variable to modify it
   - Click the trash icon to remove a variable
   - Click "Clear All" to remove all variables

2. **Bulk Edit** - Use the command palette: `Silky: Set Runtime Variables` to enter all variables as JSON

3. Variables persist across runs until cleared

---

## Context Example

The configuration

```yaml
rootContext: []

steps:
  - type: request
    name: Fetch Facilities
    request:
      url: https://www.foo-bar/GetFacilities
      method: GET
      headers:
        Accept: application/json
    resultTransformer: |
      [.Facilities[]
        | select(.ReceiptMerchant == "STA – Strutture Trasporto Alto Adige SpA Via dei Conciapelli, 60 39100  Bolzano UID: 00586190217")
      ]
    steps:
      - type: forEach
        path: .
        as: facility
        steps:
          - type: request
            name: Get Facility Free Places
            request:
              url: https://www.foo-bar/FacilityFreePlaces?FacilityID={{ .facility.FacilityId }}
              method: GET
              headers:
                Accept: application/json
            resultTransformer: '[.FreePlaces]'
            mergeOn: .FacilityDetails = $res

          - type: forEach
            path: .subFacilities
            as: sub
            steps:
              - type: request
                name: Get SubFacility Free Places
                request:
                  url: https://www.foo-bar/FacilityFreePlaces?FacilityID={{ .sub.FacilityId }}
                  method: GET
                  headers:
                    Accept: application/json
                resultTransformer: '[.FreePlaces]'
                mergeOn: .SubFacilityDetails = $res

              - type: forEach
                path: .locations
                as: loc
                steps:
                  - type: request
                    name: Get Location Details
                    request:
                      url: https://www.foo-bar/Locations/{{ .loc }}
                      method: GET
                      headers:
                        Accept: application/json
                    mergeWithContext:
                      name: sub
                      rule: ".locationDetails = (.locationDetails // {}) + {($res.id): $res}"
```

Generates a Context tree like

```
rootContext: []
│
└── Request: Fetch Facilities
    (result is filtered list of Facilities)
    │
    └── ForEach: facility in [.]
        (new child context per facility)
        │
        ├── Request: Get Facility Free Places
        │   (merges .FacilityDetails into facility context via mergeOn)
        │
        └── ForEach: sub in .subFacilities
            (new child context per sub-facility)
            │
            ├── Request: Get SubFacility Free Places
            │   (merges .SubFacilityDetails into sub context via mergeOn)
            │
            └── ForEach: loc in .locations
                (new child context per location ID)
                │
                └── Request: Get Location Details
                    (merges into sub context under .locationDetails via mergeWithContext)
```

-----

## Configuration Structure

### Top-Level Fields

| Field         | Type                   | Description                                                    |
| ------------- | ---------------------- | -------------------------------------------------------------- |
| `rootContext` | `[]` or `{}`           | **Required.** Initial context for the crawler.                 |
| `auth`        | [AuthenticationStruct](#authenticationstruct) | Optional. Global authentication configuration.                 |
| `headers`     | `map[string]string`    | Optional. Global headers applied to all requests.              |
| `stream`      | `boolean`              | Optional. Enable streaming; requires `rootContext` to be `[]`. |
| `steps`       | Array<[ForeachStep](#foreachstep)\|[ForValuesStep](#forvaluesstep)\|[RequestStep](#requeststep)> | **Required.** List of crawler steps. |

---

### AuthenticationStruct

Silky supports multiple authentication mechanisms to handle diverse API authentication patterns.

#### Common Fields

| Field          | Type   | Description                                                              |
| -------------- | ------ | ------------------------------------------------------------------------ |
| `type`         | string | **Required.** One of: `basic`, `bearer`, `oauth`, `cookie`, `jwt`, `custom` |

#### Type: `basic`

HTTP Basic Authentication.

| Field      | Type   | Required | Description          |
| ---------- | ------ | -------- | -------------------- |
| `username` | string | Yes      | Basic auth username  |
| `password` | string | Yes      | Basic auth password  |

**Example:**
```yaml
auth:
  type: basic
  username: myuser
  password: mypassword
```

#### Type: `bearer`

Bearer token authentication.

| Field   | Type   | Required | Description   |
| ------- | ------ | -------- | ------------- |
| `token` | string | Yes      | Bearer token  |

**Example:**
```yaml
auth:
  type: bearer
  token: my-api-token-123
```

#### Type: `oauth`

OAuth2 authentication with password or client credentials flow.

| Field          | Type     | Required When                                    | Description                       |
| -------------- | -------- | ------------------------------------------------ | --------------------------------- |
| `method`       | string   | Always                                           | `password` or `client_credentials`|
| `tokenUrl`     | string   | Always                                           | OAuth2 token endpoint URL         |
| `clientId`     | string   | If `method == client_credentials`                | OAuth2 client ID                  |
| `clientSecret` | string   | If `method == client_credentials`                | OAuth2 client secret              |
| `username`     | string   | If `method == password`                          | User username                     |
| `password`     | string   | If `method == password`                          | User password                     |
| `scopes`       | []string | Optional                                         | OAuth2 scopes                     |

**Example (Client Credentials):**
```yaml
auth:
  type: oauth
  method: client_credentials
  tokenUrl: https://api.example.com/oauth/token
  clientId: my-client-id
  clientSecret: my-client-secret
  scopes: [read, write]
```

**Example (Password Flow):**
```yaml
auth:
  type: oauth
  method: password
  tokenUrl: https://api.example.com/oauth/token
  username: user@example.com
  password: userpass
  scopes: [api]
```

#### Type: `cookie`

Cookie-based authentication - performs login request, extracts cookie, and injects it in subsequent requests.

| Field           | Type                             | Required | Description                              |
| --------------- | -------------------------------- | -------- | ---------------------------------------- |
| `loginRequest`  | [RequestConfig](#requeststruct)  | Yes      | Login request configuration              |
| `extractSelector` | string                         | Yes      | Cookie name to extract                   |
| `maxAgeSeconds` | int                              | Optional | Token refresh interval (0 = no refresh) |

**Example:**
```yaml
auth:
  type: cookie
  loginRequest:
    url: https://api.example.com/login
    method: POST
    headers:
      Content-Type: application/json
    body:
      username: myuser
      password: mypass
  extractSelector: session_id
  maxAgeSeconds: 3600
```

#### Type: `jwt`

JWT authentication - performs login request, extracts JWT from response, and injects as Bearer token.

| Field             | Type                            | Required | Description                                  |
| ----------------- | ------------------------------- | -------- | -------------------------------------------- |
| `loginRequest`    | [RequestConfig](#requeststruct) | Yes      | Login request configuration                  |
| `extractFrom`     | string                          | Optional | `header` or `body` (default: `body`)         |
| `extractSelector` | string                          | Yes      | Header name or jq expression for token       |
| `maxAgeSeconds`   | int                             | Optional | Token refresh interval (0 = no refresh)     |

**Example (Extract from Body):**
```yaml
auth:
  type: jwt
  loginRequest:
    url: https://api.example.com/auth/login
    method: POST
    headers:
      Content-Type: application/json
    body:
      email: user@example.com
      password: mypass
  extractFrom: body
  extractSelector: .token
  maxAgeSeconds: 3600
```

**Example (Extract from Header):**
```yaml
auth:
  type: jwt
  loginRequest:
    url: https://api.example.com/auth/login
    method: POST
    headers:
      Content-Type: application/json
    body:
      username: myuser
      password: mypass
  extractFrom: header
  extractSelector: X-Auth-Token
```

#### Type: `custom`

Fully customizable authentication - specify where to extract credentials and where to inject them.

| Field             | Type                            | Required | Description                                       |
| ----------------- | ------------------------------- | -------- | ------------------------------------------------- |
| `loginRequest`    | [RequestConfig](#requeststruct) | Yes      | Login request configuration                       |
| `extractFrom`     | string                          | Yes      | `cookie`, `header`, or `body`                     |
| `extractSelector` | string                          | Yes      | Cookie/header name or jq expression               |
| `injectInto`      | string                          | Yes      | `cookie`, `header`, `bearer`, `query`, or `body`  |
| `injectKey`       | string                          | If not bearer | Cookie/header/query/body field name          |
| `maxAgeSeconds`   | int                             | Optional | Token refresh interval (0 = no refresh)          |

**Example (Cookie to Custom Header):**
```yaml
auth:
  type: custom
  loginRequest:
    url: https://api.example.com/login
    method: POST
    headers:
      Content-Type: application/json
    body:
      username: myuser
      password: mypass
  extractFrom: cookie
  extractSelector: auth_cookie
  injectInto: header
  injectKey: X-Custom-Auth
```

**Example (Body JSON to Query Parameter):**
```yaml
auth:
  type: custom
  loginRequest:
    url: https://api.example.com/auth
    method: POST
    headers:
      Content-Type: application/json
  extractFrom: body
  extractSelector: .access_token
  injectInto: query
  injectKey: api_key
  maxAgeSeconds: 3600
```

---

### ForeachStep

Iterates over an array extracted from the current context, creating a new child context for each item.

| Field               | Type                 | Description                                          |
| ------------------- | -------------------- | ---------------------------------------------------- |
| `type`              | string               | **Required.** Must be `forEach`                      |
| `name`              | string               | Optional name for the step                           |
| `path`              | jq expression        | **Required.** Path to the array to iterate over      |
| `as`                | string               | **Required.** Context name for each item             |
| `parallelism`       | [ParallelismConfig](#parallelismconfig) | Optional. Parallel execution configuration |
| `steps`             | Array<Step>          | Optional. Nested steps to execute for each item      |
| `mergeWithParentOn` | jq expression        | Optional. Rule for merging with parent context       |
| `mergeOn`           | jq expression        | Optional. Rule for merging with current context      |
| `mergeWithContext`  | [MergeWithContextRule](#mergewithcontextrule) | Optional. Advanced merging rule |
| `noopMerge`         | bool                 | Optional. Skip merging (nested steps handle merging) |

**Note:** Only one of `mergeWithParentOn`, `mergeOn`, `mergeWithContext`, or `noopMerge` can be specified.

**Example with Parallel Execution:**
```yaml
- type: forEach
  path: .users
  as: user
  parallelism:
    maxConcurrency: 5
    requestsPerSecond: 10
    burst: 2
  steps:
    - type: request
      # ... fetch user details
```

---

### ForValuesStep

Iterates over literal values defined in the configuration, creating an overlay context for each value. Unlike `forEach`, the context variable is set directly to the value (not wrapped in an object).

| Field    | Type          | Description                                          |
| -------- | ------------- | ---------------------------------------------------- |
| `type`   | string        | **Required.** Must be `forValues`                    |
| `name`   | string        | Optional name for the step                           |
| `values` | array\<any\>  | **Required.** Literal values to iterate over         |
| `as`     | string        | **Required.** Context name for the current value     |
| `steps`  | Array<Step>   | Optional. Nested steps to execute for each value     |

**Note:** `forValues` does not support merge options or parallelism. Nested steps handle their own merging. The context variable is accessible directly (e.g., `{{ .language }}` not `{{ .language.value }}`).

**Example:**
```yaml
- type: forValues
  name: Iterate languages
  values: ["en", "de", "it"]
  as: language
  steps:
    - type: request
      request:
        url: "https://api.example.com/data?lang={{ .language }}"
        method: GET
      mergeWithContext:
        name: root
        rule: ".results += [$res]"
```

**Use Cases:**
- Iterating over a predefined set of values (languages, regions, categories)
- Matrix-style iteration when nested (e.g., regions × tiers)
- Preserving parent context variables for nested requests

---

### ParallelismConfig

Controls parallel execution of forEach iterations.

| Field               | Type    | Description                                          |
| ------------------- | ------- | ---------------------------------------------------- |
| `maxConcurrency`    | int     | Optional. Maximum concurrent workers (default: 10)   |
| `requestsPerSecond` | float64 | Optional. Maximum requests per second for rate limiting |
| `burst`             | int     | Optional. Burst size for temporary rate exceeding (default: 1) |

When `parallelism` is present on a forEach step, iterations will be executed in parallel using a worker pool. The `maxConcurrency` setting limits how many iterations run concurrently. Rate limiting is applied if `requestsPerSecond` is specified.

---

### MergeWithContextRule

| Field  | Type   | Description                                  |
| ------ | ------ | -------------------------------------------- |
| `name` | string | **Required.** Name of ancestor context       |
| `rule` | string | **Required.** jq expression for merge logic  |

---

### RequestStep

Performs an HTTP request and optionally transforms the response.

| Field               | Type          | Description                           |
| ------------------- | ------------- | ------------------------------------- |
| `type`              | string        | **Required.** Must be `request`       |
| `name`              | string        | Optional step name                    |
| `request`           | [RequestStruct](#requeststruct) | **Required.** Request configuration   |
| `resultTransformer` | jq expression | Optional transformation of the result |
| `as`                | string        | Optional. Context name for this request's result (see below) |
| `steps`             | Array<[ForeachStep](#foreachstep)\|[RequestStep](#requeststep)> | Optional. Nested steps |
| `mergeWithParentOn` | jq expression | Optional. Rule for merging with parent context |
| `mergeOn`           | jq expression | Optional. Rule for merging with current context |
| `mergeWithContext`  | [MergeWithContextRule](#mergewithcontextrule) | Optional. Advanced merging rule |

**Note:** Only one of `mergeWithParentOn`, `mergeOn`, or `mergeWithContext` can be specified.

#### Understanding the `as` Property for Requests

The `as` property on request steps creates a **new sibling context** instead of replacing the current context. This is critical when you have nested forEach loops and need inner requests to access outer forEach variables.

**The Problem: Context Replacement**

Without `forValues`, a request **replaces** the current context with its response data:

```yaml
steps:
  - type: forValues
    values: ["en", "de", "it"]
    as: language                    # Creates "language" overlay context
    steps:
      - type: request               # Creates working context with response
        request:
          url: "https://api.example.com/data?lang={{ .language }}"
        steps:
          - type: forEach
            path: .items
            as: item
            steps:
              - type: request
                request:
                  # With forValues, .language IS accessible here!
                  url: "https://api.example.com/detail?lang={{ .language }}"
```

**Alternative: Using forEach with path-based data**

When iterating over data from the context (not literal values), use `forEach`:

```yaml
steps:
  - type: forEach
    path: .languages              # Extract from context data
    as: language                  # Creates "language" context per item
    steps:
      - type: request
        request:
          url: "https://api.example.com/locations?lang={{ .language.code }}"
        steps:
          - type: forEach
            path: .
            as: location
            steps:
              - type: request
                request:
                  # Access both language and location contexts
                  url: "https://api.example.com/details?lang={{ .language.code }}&id={{ .location.id }}"
```

**When to Use `forValues` vs `forEach`:**

1. **`forValues`**: For literal values defined in configuration (languages, regions, categories)
2. **`forEach`**: For iterating over arrays extracted from context data via `path`

**Key Points:**
- `forValues` creates an overlay context that preserves parent variables - nested steps can access the value directly (e.g., `{{ .language }}`)
- `forEach` creates a child context from extracted data - access item properties (e.g., `{{ .language.code }}`)
- Use `$ctx.contextName` in jq expressions to access any named context
- Use `mergeWithContext` to merge results into canonical contexts like "root"

---

### RequestStruct

Defines an HTTP request configuration.

| Field        | Type                 | Description                      |
| ------------ | -------------------- | -------------------------------- |
| `url`        | go-template string   | **Required.** Request URL with template support |
| `method`     | string (`GET` \| `POST`) | **Required.** HTTP method |
| `headers`    | map<string, string>  | Optional headers (use `Content-Type` here for POST body type) |
| `body`       | map<string, any>     | Optional request body            |
| `pagination` | [PaginationStruct](#paginationstruct) | Optional pagination config |
| `auth`       | [AuthenticationStruct](#authenticationstruct) | Optional override authentication |

**Important:** For POST requests with a body, specify `Content-Type` in the `headers` map:

```yaml
request:
  url: https://api.example.com/data
  method: POST
  headers:
    Content-Type: application/json
  body:
    key: value
```

Supported Content-Types:
- `application/json` - Body will be JSON-encoded
- `application/x-www-form-urlencoded` - Body will be form-encoded

---

### PaginationStruct

Defines pagination behavior for requests.

| Field    | Type                          | Description                         |
| -------- | ----------------------------- | ----------------------------------- |
| `nextPageUrlSelector` | string | **Optional (either this or params).** Selector for next page URL: `body:<jq-expression>` or `header:<header-name>` |
| `params` | array<[PaginationParamsStruct](#paginationparamsstruct)> | **Optional (either this or nextPageUrlSelector).** Pagination parameters |
| `stopOn` | array<[PaginationStopsStruct](#paginationstopsstruct)>  | **Required.** Stop conditions |

**Note:** Use either `nextPageUrlSelector` for next-URL-based pagination OR `params` for offset/cursor-based pagination.

---

### PaginationParamsStruct

| Field       | Type   | Description                                                 |
| ----------- | ------ | ----------------------------------------------------------- |
| `name`      | string | **Required.** Parameter name                                |
| `location`  | string | **Required.** One of: `query`, `body`, `header`             |
| `type`      | string | **Required.** One of: `int`, `float`, `datetime`, `dynamic` |
| `format`    | string | Required if `type == datetime` (Go time format)             |
| `default`   | string | **Required.** Initial value (must match the `type`)         |
| `increment` | string | Optional. Increment expression (e.g., `+ 10`, `+1d`)        |
| `source`    | string | Required if `type == dynamic`. Format: `body:<jq-expr>` or `header:<name>` |

**Examples:**

Integer offset pagination:
```yaml
pagination:
  params:
    - name: offset
      location: query
      type: int
      default: "0"
      increment: "+ 50"
  stopOn:
    - type: pageNum
      value: 10
```

Dynamic token pagination:
```yaml
pagination:
  params:
    - name: cursor
      location: query
      type: dynamic
      source: "body:.pagination.next_cursor"
      default: ""
  stopOn:
    - type: responseBody
      expression: ".pagination.next_cursor == null"
```

---

### PaginationStopsStruct

| Field        | Type          | Description                                                         |
| ------------ | ------------- | ------------------------------------------------------------------- |
| `type`       | string        | **Required.** One of: `responseBody`, `requestParam`, `pageNum`     |
| `expression` | jq expression | Required if `type == responseBody`. Boolean jq expression           |
| `param`      | string        | Required if `type == requestParam`. Format: `.<location>.<name>`    |
| `compare`    | string        | Required if `type == requestParam`. One of: `lt`, `lte`, `eq`, `gt`, `gte` |
| `value`      | any           | Required if `type == requestParam` or `type == pageNum`             |

**Examples:**

Stop when response indicates no more pages:
```yaml
stopOn:
  - type: responseBody
    expression: ".data | length == 0"
```

Stop when offset reaches limit:
```yaml
stopOn:
  - type: requestParam
    param: .query.offset
    compare: gte
    value: 1000
```

Stop after 5 pages:
```yaml
stopOn:
  - type: pageNum
    value: 5
```

---

## Parallel Execution

Silky supports parallel execution of `forEach` iterations, significantly improving performance for I/O-bound operations.

### Configuration

```yaml
- type: forEach
  path: .items
  as: item
  parallel: true           # Enable parallel execution
  maxConcurrency: 10       # Optional: max concurrent workers (default: 10)
  rateLimit:               # Optional: rate limiting
    requestsPerSecond: 5.0
    burst: 2
  steps:
    - type: request
      # ... nested requests execute in parallel
```

### Features

- **Thread-safe merging**: All merge operations use mutexes for safe concurrent access
- **Worker pool**: Limits concurrent operations to prevent overwhelming APIs
- **Rate limiting**: Controls request rate across all workers
- **Deterministic results**: Results maintain iteration order even with parallel execution
- **Nested parallelism**: Each forEach level can have its own parallelism settings

### Best Practices

1. Use parallelism for I/O-bound operations (API calls, database queries)
2. Set appropriate `maxConcurrency` based on target API limits
3. Always configure `rateLimit` to respect API rate limits
4. Monitor for race conditions when merging to shared contexts
5. Use `noopMerge` with nested step merges for predictable ordering

---

## Stream Mode

When `stream: true` is enabled at the top-level, the crawler emits entities incrementally as it processes them. In this mode:

* `rootContext` must be an empty array (`[]`)
* Each result from `forEach` or `request` is pushed to the output stream
* Streaming happens at depth 0 or 1 in the context hierarchy
* The final result will be an empty array (data is streamed, not accumulated)

**Example:**
```yaml
rootContext: []
stream: true

steps:
  - type: request
    # ... fetches list
    steps:
      - type: forEach
        # Each item is streamed as it's processed
```

---

## Configuration Builder

The CLI utility enables real-time execution of your manifest with step-by-step inspection. It helps:

* Validate configuration
* Execute each step and inspect intermediate results
* Debug jq and template expressions interactively
* Visualize context hierarchy and data flow
* Profile execution performance

---

## Examples

The package includes several tests and examples to better understand its usage. The configuration files listed below demonstrate various features.

Feel free to contribute by adding more examples or tests! 🚀

-----

### Test Cases

These files are used for automated testing of the **paginator** and **crawler** components.

#### Paginator Tests

| Test | Description |
| :--- | :---------- |
| [`test1_int_increment.yaml`](testdata/paginator/test1_int_increment.yaml) | Pagination using simple integer increment |
| [`test2_datetime.yaml`](testdata/paginator/test2_datetime.yaml) | Pagination based on datetime values |
| [`test3_next_token.yaml`](testdata/paginator/test3_next_token.yaml) | Pagination using next token from response |
| [`test4_empty.yaml`](testdata/paginator/test4_empty.yaml) | Handling empty response |
| [`test5_empty_array.yaml`](testdata/paginator/test5_empty_array.yaml) | Handling response with empty array |
| [`test6_now_datetime.yaml`](testdata/paginator/test6_now_datetime.yaml) | Pagination using current datetime |
| [`test7_now_datetime_multistop.yaml`](testdata/paginator/test7_now_datetime_multistop.yaml) | Multiple stop conditions with datetime |
| [`test8_example_pagination_url.yaml`](testdata/paginator/test8_example_pagination_url.yaml) | Pagination using full next URL |
| [`test9_stop_on_iteration.yaml`](testdata/paginator/test9_stop_on_iteration.yaml) | Stop condition based on iteration count |

#### Crawler Tests

| Test | Description |
| :--- | :---------- |
| [`example.yaml`](testdata/crawler/example.yaml) | Baseline crawler configuration |
| [`example2.yaml`](testdata/crawler/example2.yaml) | Complex crawler with nested requests |
| [`example_single.yaml`](testdata/crawler/example_single.yaml) | Single non-paginated request |
| [`example_foreach_value.yaml`](testdata/crawler/example_foreach_value.yaml) | ForEach iteration over response values |
| [`example_foreach_value_transform_ctx.yaml`](testdata/crawler/example_foreach_value_transform_ctx.yaml) | ForEach with context in transformation |
| [`example_foreach_value_stream.yaml`](testdata/crawler/example_foreach_value_stream.yaml) | ForEach iteration with streaming |
| [`example_pagination_next.yaml`](testdata/crawler/example_pagination_next.yaml) | Pagination using next_url from response |
| [`example_pagination_increment.yaml`](testdata/crawler/example_pagination_increment.yaml) | Pagination with incrementing number |
| [`example_pagination_increment_stream.yaml`](testdata/crawler/example_pagination_increment_stream.yaml) | Pagination with streaming enabled |
| [`example_pagination_increment_nested.yaml`](testdata/crawler/example_pagination_increment_nested.yaml) | Pagination on nested request |
| [`post_json_body.yaml`](testdata/crawler/post_json_body.yaml) | POST request with JSON body |
| [`post_form_urlencoded.yaml`](testdata/crawler/post_form_urlencoded.yaml) | POST request with form-encoded body |
| [`post_body_merge_pagination.yaml`](testdata/crawler/post_body_merge_pagination.yaml) | POST with body, pagination, and merging |

#### Authentication Tests

| Test | Description |
| :--- | :---------- |
| [`auth_basic.yaml`](testdata/crawler/auth_basic.yaml) | Basic HTTP authentication |
| [`auth_bearer.yaml`](testdata/crawler/auth_bearer.yaml) | Bearer token authentication |
| [`auth_oauth_password.yaml`](testdata/crawler/auth_oauth_password.yaml) | OAuth2 password grant flow |
| [`auth_oauth_client_credentials.yaml`](testdata/crawler/auth_oauth_client_credentials.yaml) | OAuth2 client credentials flow |
| [`auth_cookie.yaml`](testdata/crawler/auth_cookie.yaml) | Cookie-based authentication |
| [`auth_jwt_body.yaml`](testdata/crawler/auth_jwt_body.yaml) | JWT auth with token from response body |
| [`auth_jwt_header.yaml`](testdata/crawler/auth_jwt_header.yaml) | JWT auth with token from response header |
| [`auth_custom_cookie_to_header.yaml`](testdata/crawler/auth_custom_cookie_to_header.yaml) | Custom auth: extract cookie, inject as header |
| [`auth_custom_body_to_query.yaml`](testdata/crawler/auth_custom_body_to_query.yaml) | Custom auth: extract from body, inject as query param |
| [`auth_mixed_override.yaml`](testdata/crawler/auth_mixed_override.yaml) | Global auth with request-level override |

#### ForValues Tests

| Test | Description |
| :--- | :---------- |
| [`forvalues_simple.yaml`](testdata/crawler/forvalues_simple.yaml) | Simple forValues iteration |
| [`forvalues_nested.yaml`](testdata/crawler/forvalues_nested.yaml) | Nested forValues iterations |
| [`forvalues_objects.yaml`](testdata/crawler/forvalues_objects.yaml) | ForValues with object values |
| [`request_as_dynamic_keys.yaml`](testdata/crawler/request_as_dynamic_keys.yaml) | Request with dynamic context keys |
| [`request_as_context_disconnect.yaml`](testdata/crawler/request_as_context_disconnect.yaml) | Request context isolation |
| [`edge_case_multiple_forvalues.yaml`](testdata/crawler/edge_case_multiple_forvalues.yaml) | Multiple forValues at same level |
| [`edge_case_deep_nesting.yaml`](testdata/crawler/edge_case_deep_nesting.yaml) | Deep nesting edge cases |

#### Parallel Execution Tests

| Test | Description |
| :--- | :---------- |
| [`parallel/simple.yaml`](testdata/crawler/parallel/simple.yaml) | Basic parallel forEach execution |
| [`parallel/ratelimited.yaml`](testdata/crawler/parallel/ratelimited.yaml) | Parallel execution with rate limiting |
| [`parallel/noop_merge.yaml`](testdata/crawler/parallel/noop_merge.yaml) | Parallel with noopMerge strategy |
| [`parallel/nested_parallel.yaml`](testdata/crawler/parallel/nested_parallel.yaml) | Nested parallel forEach steps |
| [`parallel/multi_root_parallel.yaml`](testdata/crawler/parallel/multi_root_parallel.yaml) | Multiple parallel roots |
| [`parallel/error_handling.yaml`](testdata/crawler/parallel/error_handling.yaml) | Error handling in parallel execution |

-----

### Usage Examples

These files provide practical, ready-to-use examples for common crawling patterns.

| Example | Description |
| :------ | :---------- |
| [`foreach-iteration-not-streamed.yaml`](examples/foreach-iteration-not-streamed.yaml) | Iterating over a list without streaming |
| [`list-and-details-paginated-stopped-streamed.yaml`](examples/list-and-details-paginated-stopped-streamed.yaml) | Pagination + stop conditions + streaming |
| [`pagination-url-not-stream.yaml`](examples/pagination-url-not-stream.yaml) | Pagination using next URL without streaming |

-----

## Debug & Development

### Running Tests
```bash
# Run all tests
go test -v ./...

# Run specific test
go test -v -run TestExampleForeachValue

# Run paginator tests only
go test -v -run TestPaginator
```

### Building
```bash
# Build all packages
go build -v ./...

# Build the IDE
cd cmd/ide && go build -o silky-ide
```

### Debugging with Delve
```bash
# Debug IDE with headless Delve (attach from VS Code or other debugger)
cd cmd/ide && dlv debug ./... --headless=true --listen=:2345 --api-version=2
```

### VS Code Debug Configuration
The repository includes VS Code debug configurations in `.vscode/launch.json` for attaching to the Delve debugger on port 2345.
