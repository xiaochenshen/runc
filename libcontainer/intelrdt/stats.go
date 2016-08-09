// +build linux

package intelrdt

// The read-only stats in root of Intel RDT "resource control" filesystem
type IntelRdtRootStats struct {
	Info              string `json:"info,omitempty"`
	DomainToCacheId   string `json:"domain_to_cache_id,omitempty"`
	MaxCbmLen         uint64 `json:"max_cbm_len,omitempty"`
	MaxClosid         uint64 `json:"max_closid,omitempty"`
	RootL3CacheSchema string `json:"root_l3_cache_schema,omitempty"`
	RootL3CacheCpus   string `json:"root_l3_cache_cpus,omitempty"`
}

type IntelRdtSubStats struct {
	L3CacheSchema string `json:"l3_cache_schema,omitempty"`
	L3CacheCpus   string `json:"l3_cache_cpus,omitempty"`
}

type Stats struct {
	IntelRdtRootStats IntelRdtRootStats `json:"intel_rdt_root_stats,omitempty"`
	IntelRdtSubStats  IntelRdtSubStats  `json:"intel_rdt_sub_stats,omitempty"`
}

func NewStats() *Stats {
	return &Stats{}
}
