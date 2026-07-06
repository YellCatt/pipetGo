package psv

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.uber.org/zap"

	"pipet/internal/logger"
)

type StreamAssert struct {
	Kind      string `json:"kind"`
	Pattern   string `json:"pattern"`
	MaxWaitMs int    `json:"max_wait_ms"`
	MinChunks int    `json:"min_chunks"`
}

type TestCase struct {
	ID             string            `mapstructure:"id"`
	Skip           bool              `mapstructure:"skip"`
	Desc           string            `mapstructure:"desc"`
	Method         string            `mapstructure:"method"`
	URL            string            `mapstructure:"url"`
	Headers        map[string]string `mapstructure:"headers"`
	Params         map[string]string `mapstructure:"params"`
	Form           map[string]string `mapstructure:"form"`
	JSON           string            `mapstructure:"json"`
	Body           string            `mapstructure:"body"`
	Payload        string            `mapstructure:"payload"`
	ExpectedStatus int               `mapstructure:"expected_status"`
	ExpectedBody   string            `mapstructure:"expected_body"`
	Tags           []string          `mapstructure:"tags"`
	Extract        string            `mapstructure:"extract"`
	StreamMode     bool              `mapstructure:"stream_mode"`
	StreamAssert   []StreamAssert    `mapstructure:"stream_assert"`
	MatchMode      string            `mapstructure:"match_mode"`
	BodyRegex      string            `mapstructure:"body_regex"`
	Pre            []string          `mapstructure:"pre"`
	Post           []string          `mapstructure:"post"`
}

func ParseFile(filePath string) ([]TestCase, error) {
	file, err := os.Open(filePath)
	if err != nil {
		logger.Error("Failed to open PSV file", zap.String("path", filePath), zap.Error(err))
		return nil, err
	}
	defer file.Close()

	return parseReader(file, filePath)
}

func ParseFiles(paths []string) ([]TestCase, error) {
	var allCases []TestCase
	for _, path := range paths {
		files, err := expandPath(path)
		if err != nil {
			logger.Warn("Failed to expand path", zap.String("path", path), zap.Error(err))
			continue
		}

		for _, file := range files {
			cases, err := ParseFile(file)
			if err != nil {
				logger.Error("Failed to parse PSV file", zap.String("path", file), zap.Error(err))
				continue
			}
			allCases = append(allCases, cases...)
		}
	}
	return allCases, nil
}

func expandPath(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		files, err := doublestar.Glob(path)
		if err != nil {
			return nil, err
		}
		return files, nil
	}

	if info.IsDir() {
		return doublestar.Glob(filepath.Join(path, "*.psv"))
	}
	return []string{path}, nil
}

func parseReader(reader io.Reader, filePath string) ([]TestCase, error) {
	var testCases []TestCase
	scanner := bufio.NewScanner(reader)
	var header []string
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if lineNum == 1 {
			header = parseLine(line)
			continue
		}

		fields := parseLine(line)
		tc, err := parseTestCase(header, fields)
		if err != nil {
			logger.Warn("Failed to parse test case", zap.String("file", filePath), zap.Int("line", lineNum), zap.Error(err))
			continue
		}
		testCases = append(testCases, tc)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading PSV file", zap.String("file", filePath), zap.Error(err))
		return nil, err
	}

	logger.Info("Successfully parsed PSV file", zap.String("path", filePath), zap.Int("count", len(testCases)))
	return testCases, nil
}

func parseLine(line string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false

	for _, char := range line {
		switch char {
		case '"':
			inQuotes = !inQuotes
		case '|':
			if !inQuotes {
				fields = append(fields, current.String())
				current.Reset()
				continue
			}
			current.WriteRune(char)
		default:
			current.WriteRune(char)
		}
	}
	fields = append(fields, current.String())

	for i, f := range fields {
		fields[i] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(f, "\""), "\""))
	}

	return fields
}

func parseTestCase(header []string, fields []string) (TestCase, error) {
	tc := TestCase{
		Headers:   make(map[string]string),
		Params:    make(map[string]string),
		Form:      make(map[string]string),
		MatchMode: "exact",
	}

	for i, h := range header {
		if i >= len(fields) {
			continue
		}
		value := fields[i]
		trimmedHeader := strings.ToLower(strings.TrimSpace(h))

		switch trimmedHeader {
		case "id":
			tc.ID = value
		case "skip":
			tc.Skip = value == "1" || strings.EqualFold(value, "true")
		case "desc":
			tc.Desc = value
		case "method":
			tc.Method = strings.ToUpper(value)
		case "url":
			tc.URL = value
		case "headers":
			tc.Headers = parseKeyValueMap(value)
		case "params":
			tc.Params = parseKeyValueMap(value)
		case "form":
			tc.Form = parseKeyValueMap(value)
		case "json":
			tc.JSON = value
		case "body":
			tc.Body = value
		case "payload":
			tc.Payload = value
		case "expected_status":
			tc.ExpectedStatus = parseInt(value)
		case "expected_body":
			tc.ExpectedBody = value
		case "tags":
			tc.Tags = parseTags(value)
		case "extract":
			tc.Extract = value
		case "stream_mode":
			tc.StreamMode = value == "1" || strings.EqualFold(value, "true")
		case "stream_assert":
			if value != "" {
				json.Unmarshal([]byte(value), &tc.StreamAssert)
			}
		case "match_mode":
			tc.MatchMode = value
		case "body_regex":
			tc.BodyRegex = value
		case "pre":
			tc.Pre = parseDelimited(value, ";")
		case "post":
			tc.Post = parseDelimited(value, ";")
		}
	}

	if tc.ID == "" {
		tc.ID = generateID(tc)
	}

	return tc, nil
}

func parseKeyValueMap(str string) map[string]string {
	m := make(map[string]string)
	if str == "" || str == "{}" {
		return m
	}

	str = strings.TrimPrefix(strings.TrimSuffix(str, "}"), "{")
	pairs := strings.Split(str, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(strings.Trim(kv[0], "\"'"))
			value := strings.TrimSpace(strings.Trim(kv[1], "\"'"))
			m[key] = value
		}
	}
	return m
}

func parseTags(str string) []string {
	if str == "" {
		return nil
	}
	tags := strings.Split(str, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	return tags
}

func parseDelimited(str string, delimiter string) []string {
	if str == "" {
		return nil
	}
	parts := strings.Split(str, delimiter)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

func parseInt(s string) int {
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(s)
	if match == "" {
		return 0
	}
	var result int
	for _, c := range match {
		result = result*10 + int(c-'0')
	}
	return result
}

func generateID(tc TestCase) string {
	base := strings.ToLower(tc.Method) + "_" + strings.ReplaceAll(strings.ToLower(tc.URL), "/", "_")
	base = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(base, "")
	return base
}
