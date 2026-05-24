package model

// CacheSimResult — полный результат CacheSim для одного .c файла.
type CacheSimResult struct {
	SourceFile   string             `json:"source_file"`
	SimTimeSec   float64            `json:"sim_time_sec"`
	L1           CacheLevelSummary  `json:"l1"`
	L2           CacheLevelSummary  `json:"l2"`
	L3           CacheLevelSummary  `json:"l3"`
	Arrays       []ArrayCacheMetric `json:"arrays"`
	MemoryReads  uint64             `json:"memory_reads"`
	MemoryWrites uint64             `json:"memory_writes"`
}

// CacheLevelSummary — агрегированные метрики одного уровня кэша (L1/L2).
type CacheLevelSummary struct {
	CacheLevel    string  `json:"cache_level"`
	CacheSizeKB   uint32  `json:"cache_size_kb"`
	CacheLineSize uint32  `json:"cache_line_size"`
	Associativity uint8   `json:"associativity"`
	TotalAccesses uint64  `json:"total_accesses"`
	TotalHits     uint64  `json:"total_hits"`
	TotalMisses   uint64  `json:"total_misses"`
	HitsRead      uint64  `json:"hits_read"`
	HitsWrite     uint64  `json:"hits_write"`
	MissesRead    uint64  `json:"misses_read"`
	MissesWrite   uint64  `json:"misses_write"`
	MissRate      float64 `json:"miss_rate"`
}

// ArrayCacheMetric — промахи по конкретному массиву для конкретного уровня кэша.
// Это данные для JOIN с static_patterns по (source_file, array_name = base_symbol).
type ArrayCacheMetric struct {
	CacheLevel  string `json:"cache_level"`
	ArrayName   string `json:"array_name"`
	MissesTotal uint64 `json:"misses_total"`
	MissesRead  uint64 `json:"misses_read"`
	MissesWrite uint64 `json:"misses_write"`
}

func (r CacheSimResult) CacheLevels() []CacheLevelSummary {
	levels := make([]CacheLevelSummary, 0, 3)
	for _, level := range []CacheLevelSummary{r.L1, r.L2, r.L3} {
		if level.CacheLevel == "" {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}

// StaticPattern — паттерн из статического анализа (для оценки кэш-поведения).
type StaticPattern struct {
	AccessKind      string   `json:"access_kind"`
	Affine          bool     `json:"affine"`
	BaseSymbol      string   `json:"base_symbol"`
	Depth           int      `json:"depth"`
	FillFactor      float64  `json:"fill_factor"`
	Function        string   `json:"function"`
	HasIndexedAddr  bool     `json:"has_indexed_addressing"`
	IndexedByMemory bool     `json:"indexed_by_memory"`
	PatternType     string   `json:"pattern_type"`
	SourceFile      string   `json:"source_file"`
	SourceLine      int      `json:"source_line"`
	Stride          *float64 `json:"stride"`
}
