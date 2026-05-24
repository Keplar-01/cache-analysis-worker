package interpreter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/diploma/worker-cache-interpreter/internal/model"
)

// Interpreter — обёртка над cats cache simulator.
type Interpreter struct {
	binaryPath string
	timeoutSec int
}

func New(binaryPath string, timeoutSec int) *Interpreter {
	return &Interpreter{binaryPath: binaryPath, timeoutSec: timeoutSec}
}

// Run запускает cats над .c файлом и читает JSON-артефакт *_result.json.
func (i *Interpreter) Run(ctx context.Context, sourceFile, configFile string) (*model.CacheSimResult, error) {
	var stdout, stderr bytes.Buffer
	if i.timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(i.timeoutSec)*time.Second)
		defer cancel()
	}

	args := []string{filepath.Base(sourceFile), "json"}
	if strings.TrimSpace(configFile) != "" {
		args = append(args, filepath.Base(configFile))
	}

	cmd := exec.CommandContext(ctx, i.binaryPath, args...)
	cmd.Dir = filepath.Dir(sourceFile)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if i.timeoutSec > 0 && ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("cachesim exceeded timeout %ds, stderr: %s", i.timeoutSec, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("cachesim exec failed: %w, stderr: %s", err, stderr.String())
	}

	rawOutput, err := i.readResult(sourceFile)
	if err != nil {
		return nil, err
	}

	result, err := parseJSONOutput(rawOutput)
	if err != nil {
		return nil, fmt.Errorf("parse cachesim JSON output: %w", err)
	}

	result.SourceFile = filepath.Base(sourceFile)
	return result, nil
}

func (i *Interpreter) readResult(sourceFile string) (string, error) {
	resultFile := resultFilePath(sourceFile)
	if payload, err := os.ReadFile(resultFile); err == nil {
		trimmed := strings.TrimSpace(string(payload))
		if trimmed != "" {
			return trimmed, nil
		}
	}

	if fallbackFile, ok := findFallbackResultFile(sourceFile); ok {
		if payload, err := os.ReadFile(fallbackFile); err == nil {
			trimmed := strings.TrimSpace(string(payload))
			if trimmed != "" {
				return trimmed, nil
			}
		}
	}

	return "", fmt.Errorf("cachesim produced no JSON result file %s", filepath.Base(resultFile))
}

func resultFilePath(sourceFile string) string {
	name := filepath.Base(sourceFile)
	ext := filepath.Ext(name)
	if ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return filepath.Join(filepath.Dir(sourceFile), name+"_result.json")
}

func findFallbackResultFile(sourceFile string) (string, bool) {
	entries, err := os.ReadDir(filepath.Dir(sourceFile))
	if err != nil {
		return "", false
	}

	var resultFile string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_result.json") {
			continue
		}
		if resultFile != "" {
			return "", false
		}
		resultFile = filepath.Join(filepath.Dir(sourceFile), entry.Name())
	}

	if resultFile == "" {
		return "", false
	}
	return resultFile, true
}

// --- Парсинг stdout CacheSim ---
//
// Формат вывода:
//   Time is 1.44955
//
//   Cache L1
//   Cache size 32 kB 8-way
//   Cache line size 64
//   Cache access: 4003000
//   Cache hit: 4002812 (write - 1002812 , read - 3000000)
//   Cache misses: 188 (write - 188 , read - 0)
//   Missrate: 0.00469648
//   Cache misses array a: 62 (write - 62 , read - 0)
//   Cache misses array b: 63 (write - 63 , read - 0)
//   Cache misses array c: 63 (write - 63 , read - 0)
//
//   Cache L2
//   ... (аналогично)
//
//   Memory reads: 188
//   Memory writes: 188

var (
	reTime       = regexp.MustCompile(`Time is ([\d.]+)`)
	reCacheLevel = regexp.MustCompile(`^Cache (L[123])$`)
	reCacheSize  = regexp.MustCompile(`Cache size (\d+) kB (\d+)-way`)
	reLineSize   = regexp.MustCompile(`Cache line size (\d+)`)
	reAccess     = regexp.MustCompile(`^Cache access: (\d+)`)
	reHit        = regexp.MustCompile(`^Cache hit: (\d+) \(write - (\d+) , read - (\d+)\)`)
	reMiss       = regexp.MustCompile(`^Cache misses: (\d+) \(write - (\d+) , read - (\d+)\)`)
	reMissRate   = regexp.MustCompile(`^Missrate: ([\d.eE+\-nan]+)`)
	reMissArray  = regexp.MustCompile(`^Cache misses array (\w+): (\d+) \(write - (\d+) , read - (\d+)\)`)
	reMemReads   = regexp.MustCompile(`^Memory reads: (\d+)`)
	reMemWrites  = regexp.MustCompile(`^Memory writes: (\d+)`)
)

func parseOutput(raw string) (*model.CacheSimResult, error) {
	if payload, ok := jsonPayload(raw); ok {
		return parseJSONOutput(payload)
	}
	return parseTextOutput(raw)
}

func jsonPayload(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return trimmed, true
	}

	objectIndex := strings.Index(trimmed, "{")
	arrayIndex := strings.Index(trimmed, "[")
	switch {
	case objectIndex == -1 && arrayIndex == -1:
		return "", false
	case objectIndex == -1:
		return trimmed[arrayIndex:], true
	case arrayIndex == -1:
		return trimmed[objectIndex:], true
	case objectIndex < arrayIndex:
		return trimmed[objectIndex:], true
	default:
		return trimmed[arrayIndex:], true
	}
}

type cacheJSONLevel struct {
	LevelName      string
	CacheSize      uint64
	CacheBlockSize uint64
	Way            uint64
	HitRead        uint64
	HitWrite       uint64
	MissRead       uint64
	MissWrite      uint64
	Arrays         map[string]*model.ArrayCacheMetric
}

func parseJSONOutput(raw string) (*model.CacheSimResult, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	result := &model.CacheSimResult{}
	parsedAny := false
	for {
		var value any
		if err := decoder.Decode(&value); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		parsedAny = true
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				levelMap, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unexpected JSON array item %T", item)
				}
				applyJSONLevel(result, parseJSONLevel(levelMap))
			}
		case map[string]any:
			applyJSONLevel(result, parseJSONLevel(typed))
		default:
			return nil, fmt.Errorf("unexpected JSON value %T", value)
		}
	}

	if !parsedAny {
		return nil, fmt.Errorf("empty CacheSim JSON output")
	}
	if result.L1.CacheLevel == "" {
		return nil, fmt.Errorf("failed to parse L1 cache block from CacheSim JSON output")
	}
	levels := result.CacheLevels()
	deepest := levels[len(levels)-1]
	result.MemoryReads = deepest.MissesRead
	result.MemoryWrites = deepest.MissesWrite
	return result, nil
}

func parseJSONLevel(raw map[string]any) cacheJSONLevel {
	level := cacheJSONLevel{Arrays: make(map[string]*model.ArrayCacheMetric)}
	for key, value := range raw {
		switch key {
		case "level_name":
			level.LevelName = fmt.Sprint(value)
		case "cacheSize":
			level.CacheSize = jsonUint(value)
		case "cacheBlockSize":
			level.CacheBlockSize = jsonUint(value)
		case "way":
			level.Way = jsonUint(value)
		case "hit_read":
			level.HitRead = jsonUint(value)
		case "hit_write":
			level.HitWrite = jsonUint(value)
		case "miss_read":
			level.MissRead = jsonUint(value)
		case "miss_write":
			level.MissWrite = jsonUint(value)
		default:
			if strings.HasPrefix(key, "array ") {
				parseJSONArrayMetric(level.Arrays, key, jsonUint(value))
			}
		}
	}
	return level
}

func parseJSONArrayMetric(arrays map[string]*model.ArrayCacheMetric, key string, value uint64) {
	nameAndKind := strings.TrimPrefix(key, "array ")
	var name, kind string
	switch {
	case strings.HasSuffix(nameAndKind, "_read"):
		name = strings.TrimSuffix(nameAndKind, "_read")
		kind = "read"
	case strings.HasSuffix(nameAndKind, "_write"):
		name = strings.TrimSuffix(nameAndKind, "_write")
		kind = "write"
	default:
		return
	}
	if name == "" {
		return
	}

	metric := arrays[name]
	if metric == nil {
		metric = &model.ArrayCacheMetric{ArrayName: name}
		arrays[name] = metric
	}
	if kind == "read" {
		metric.MissesRead = value
	} else {
		metric.MissesWrite = value
	}
	metric.MissesTotal = metric.MissesRead + metric.MissesWrite
}

func applyJSONLevel(result *model.CacheSimResult, level cacheJSONLevel) {
	levelName := strings.ToUpper(strings.TrimSpace(level.LevelName))
	summary := model.CacheLevelSummary{
		CacheLevel:    levelName,
		CacheSizeKB:   cacheSizeKB(level.CacheSize),
		CacheLineSize: uint32(level.CacheBlockSize),
		Associativity: uint8(level.Way),
		HitsRead:      level.HitRead,
		HitsWrite:     level.HitWrite,
		MissesRead:    level.MissRead,
		MissesWrite:   level.MissWrite,
	}
	summary.TotalHits = summary.HitsRead + summary.HitsWrite
	summary.TotalMisses = summary.MissesRead + summary.MissesWrite
	summary.TotalAccesses = summary.TotalHits + summary.TotalMisses
	if summary.TotalAccesses > 0 {
		summary.MissRate = float64(summary.TotalMisses) / float64(summary.TotalAccesses)
	}

	switch levelName {
	case "L1":
		result.L1 = summary
	case "L2":
		result.L2 = summary
	case "L3":
		result.L3 = summary
	}

	names := make([]string, 0, len(level.Arrays))
	for name := range level.Arrays {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		metric := *level.Arrays[name]
		metric.CacheLevel = levelName
		result.Arrays = append(result.Arrays, metric)
	}
}

func cacheSizeKB(size uint64) uint32 {
	if size == 0 {
		return 0
	}
	if size < 1024 {
		return uint32(size)
	}
	return uint32(size / 1024)
}

func jsonUint(value any) uint64 {
	switch v := value.(type) {
	case json.Number:
		parsed, _ := strconv.ParseUint(v.String(), 10, 64)
		return parsed
	case float64:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case string:
		return parseUint(v)
	default:
		return 0
	}
}

func parseTextOutput(raw string) (*model.CacheSimResult, error) {
	result := &model.CacheSimResult{}
	var currentLevel *model.CacheLevelSummary

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if m := reTime.FindStringSubmatch(line); m != nil {
			result.SimTimeSec = parseFloat(m[1])
			continue
		}

		if m := reCacheLevel.FindStringSubmatch(line); m != nil {
			level := m[1]
			switch level {
			case "L1":
				result.L1.CacheLevel = "L1"
				currentLevel = &result.L1
			case "L2":
				result.L2.CacheLevel = "L2"
				currentLevel = &result.L2
			default:
				result.L3.CacheLevel = "L3"
				currentLevel = &result.L3
			}
			continue
		}

		if currentLevel == nil {
			// Глобальные метрики (после блоков L1/L2)
			if m := reMemReads.FindStringSubmatch(line); m != nil {
				result.MemoryReads = parseUint(m[1])
			}
			if m := reMemWrites.FindStringSubmatch(line); m != nil {
				result.MemoryWrites = parseUint(m[1])
			}
			continue
		}

		if m := reCacheSize.FindStringSubmatch(line); m != nil {
			currentLevel.CacheSizeKB = uint32(parseUint(m[1]))
			currentLevel.Associativity = uint8(parseUint(m[2]))
			continue
		}
		if m := reLineSize.FindStringSubmatch(line); m != nil {
			currentLevel.CacheLineSize = uint32(parseUint(m[1]))
			continue
		}
		if m := reAccess.FindStringSubmatch(line); m != nil {
			currentLevel.TotalAccesses = parseUint(m[1])
			continue
		}
		if m := reHit.FindStringSubmatch(line); m != nil {
			currentLevel.TotalHits = parseUint(m[1])
			currentLevel.HitsWrite = parseUint(m[2])
			currentLevel.HitsRead = parseUint(m[3])
			continue
		}
		if m := reMissArray.FindStringSubmatch(line); m != nil {
			result.Arrays = append(result.Arrays, model.ArrayCacheMetric{
				CacheLevel:  currentLevel.CacheLevel,
				ArrayName:   m[1],
				MissesTotal: parseUint(m[2]),
				MissesWrite: parseUint(m[3]),
				MissesRead:  parseUint(m[4]),
			})
			continue
		}
		if m := reMiss.FindStringSubmatch(line); m != nil {
			currentLevel.TotalMisses = parseUint(m[1])
			currentLevel.MissesWrite = parseUint(m[2])
			currentLevel.MissesRead = parseUint(m[3])
			continue
		}
		if m := reMissRate.FindStringSubmatch(line); m != nil {
			currentLevel.MissRate = parseFloat(m[1])
			continue
		}

		// После блока L2 переходим к глобальным метрикам
		if m := reMemReads.FindStringSubmatch(line); m != nil {
			result.MemoryReads = parseUint(m[1])
			currentLevel = nil
		}
		if m := reMemWrites.FindStringSubmatch(line); m != nil {
			result.MemoryWrites = parseUint(m[1])
			currentLevel = nil
		}
	}

	if result.L1.CacheLevel == "" {
		return nil, fmt.Errorf("failed to parse L1 cache block from CacheSim output")
	}

	return result, nil
}

func parseUint(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
