# pipet

A powerful enterprise-grade API testing tool written in Go.

## Features

- RESTful API testing
- PSV (Pipe-Separated Values) test case management
- Configuration management via YAML
- Structured logging with Zap
- Parallel test execution
- Test reporting
- Tag-based test filtering
- Variable extraction and replacement
- Stream (SSE) assertion support
- Regex assertion support

## Requirements

- Go 1.22+

## Installation

```bash
go mod download
go build -o pipet.exe
```

## Configuration

Edit `config/config.yaml`:

```yaml
target:
  base_url: "https://httpbin.org"
  timeout: 30

log:
  level: "info"
  encoding: "json"
  output: "stdout"

test:
  report_dir: "./reports"
  test_case_dir: "./testcases"
```

## Usage

### Run all tests from default directory

```bash
./pipet.exe
```

### Run specific PSV files

```bash
./pipet.exe tests/test_data.psv tests/test_data2.psv
```

### Run tests from directory

```bash
./pipet.exe tests
```

### Tag filtering

```bash
# Run only smoke tests
./pipet.exe --tags=smoke

# Run smoke and api tests
./pipet.exe --tags=smoke,api
```

## PSV Test Case Format

```psv
id|skip|desc|method|url|headers|params|form|json|body|expected_status|expected_body|tags|extract|stream_mode|stream_assert|match_mode|body_regex|pre|post
```

### Columns

| Column | Description |
|--------|-------------|
| `id` | Test case unique identifier |
| `skip` | Skip test (0/1 or true/false) |
| `desc` | Test case description |
| `method` | HTTP method (GET, POST, PUT, DELETE, PATCH, HEAD) |
| `url` | API endpoint URL |
| `headers` | Request headers as JSON object |
| `params` | URL query parameters |
| `form` | Form data |
| `json` | JSON request body |
| `body` | Raw request body |
| `payload` | Compatibility field for raw body |
| `expected_status` | Expected HTTP status code |
| `expected_body` | Expected JSON response |
| `tags` | Comma-separated tags for filtering |
| `extract` | Extract variables from response (e.g., `var=path`) |
| `stream_mode` | Enable SSE streaming mode (0/1) |
| `stream_assert` | Stream assertion rules JSON array |
| `match_mode` | `exact` (default) or `subset` matching |
| `body_regex` | Regex pattern for entire response body |
| `pre` | Pre-condition test IDs (semicolon-separated) |
| `post` | Post-condition test IDs (semicolon-separated) |

### Example

```psv
get_ip|0|获取IP地址|GET|{{base_url}}/ip|{}|||{}||200|{"origin":"{{regex:^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$}}"}|api|ip=origin|||exact|||
post_json|0|POST JSON|POST|{{base_url}}/post|{}|||{"name":"test"}||200|{"json":{"name":"test"}}|api|||subset|||
```

## Variable Replacement

All fields support `{{var}}` variable replacement:

```psv
# Define base_url in config or extract from response
get_users|0|获取用户列表|GET|{{base_url}}/users|{}|||{}||200|{}|api|||
```

### Extraction

Extract variables from responses:

```psv
# Extract user_id from response
create_user|0|创建用户|POST|{{base_url}}/users|{}|||{"name":"test"}||201|{}|api|user_id=id|||

# Use extracted variable
get_user|0|获取用户|GET|{{base_url}}/users/{{user_id}}|{}|||{}||200|{}|api|||
```

## Regex Assertions

### Field-level regex

```psv
id|skip|desc|method|url|expected_status|expected_body|tags
regex_01|0|检查IP格式|GET|{{base_url}}/ip|200|{"origin":"{{regex:^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$}}"}|regex
regex_02|0|检查无错误|GET|{{base_url}}/status/200|200|{"message":"{{not_regex:error}}"}|regex
```

### Response body regex

```psv
id|skip|desc|method|url|expected_status|body_regex|tags
body_01|0|确保响应无错误|GET|{{base_url}}/health|200|!error|health
body_02|0|确保包含成功|GET|{{base_url}}/success|200|success|health
```

### Special markers

| Marker | Description |
|--------|-------------|
| `{{regex:...}}` | Field value must match regex |
| `{{not_regex:...}}` | Field value must NOT match regex |
| `{{skip}}` | Skip this field check |
| `{{not_exists}}` | Field must NOT exist in response |

## Match Modes

### Exact match (default)

Response JSON must exactly match `expected_body`:

```psv
strict_01|0|严格匹配|GET|{{base_url}}/ip|200|{"origin":"{{regex:^[0-9]{1,3}\\..*}}"}|strict|
```

### Subset match

Response JSON must contain at least the fields in `expected_body`:

```psv
subset_01|0|子集匹配|GET|{{base_url}}/get|200|{"args":{}}|api|subset|
```

## Stream Assertion

For SSE streaming responses:

```psv
stream_01|0|SSE流式断言|POST|{{base_url}}/chat/completions|{"Content-Type":"application/json"}|||{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],"stream":true}||200|{"aggregated_content":"{{regex:.*hi.*}}","chunk_count":"{{skip}}"}|stream||1|[{"kind":"contains","pattern":"hi","min_chunks":1}]|subset|||
```

### Stream assertion rules

| Field | Description |
|-------|-------------|
| `kind` | `contains`, `regex`, or `json_path` |
| `pattern` | Assertion pattern |
| `max_wait_ms` | Max wait time (reserved) |
| `min_chunks` | Minimum chunks required |

## Report Generation

After each run, reports are saved to `reports/`:

- `report_YYYYMMDD_HHMMSS.psv` - Full test results
- `report_YYYYMMDD_HHMMSS_error.psv` - Failed test cases only

## Dependencies

- `github.com/go-resty/resty/v2` - HTTP client
- `github.com/spf13/viper` - Configuration management
- `github.com/tidwall/gjson` - JSON parsing
- `go.uber.org/zap` - Logging
- `github.com/bmatcuk/doublestar/v4` - File globbing

## License

MIT