// +build linux

package intelrdt

type IntelRdtRootStats struct {
	L3CacheSchema string `json:"l3_cache_schema,omitempty"`
}

type IntelRdtStats struct {
	L3CacheSchema string `json:"l3_cache_schema,omitempty"`
}

type Stats struct {
	IntelRdtRootStats IntelRdtRootStats `json:"intel_rdt_root_stats,omitempty"`
	IntelRdtStats     IntelRdtStats     `json:"intel_rdt_stats,omitempty"`
}

func NewStats() *Stats {
	return &Stats{}
}
