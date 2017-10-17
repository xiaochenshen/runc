package configs

import "fmt"

type MemBwSchema struct {
	CacheId      uint32 `json:"cache_id"`
	BwPercentage uint32 `json:"bw_percentage"`
}

type IntelRdt struct {
	// The schema for L3 cache id and capacity bitmask (CBM)
	// Format: "L3:<cache_id0>=<cbm0>;<cache_id1>=<cbm1>;..."
	L3CacheSchema string `json:"l3_cache_schema,omitempty"`

	// The schema of memory bandwidth (b/w) percentage per L3 cache id
	// Format: "MB:<cache_id0>=bandwidth0;<cache_id1>=bandwidth1;..."
	MemBwSchema []*MemBwSchema `json:"mem_bw_schema,omitempty"`
}

// MemBwSchemaString formats the struct to be writable to the Intel RDT specific file
func (m *MemBwSchema) MemBwString() string {
	return fmt.Sprintf("%d=%d", m.CacheId, m.BwPercentage)
}
