// +build linux

package intelrdt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/opencontainers/runc/libcontainer/configs"
)

type Manager interface {
	// Applies configuration to the process with the specified pid
	Apply(pid int) error

	// Returns the PIDs inside Intel RDT "resource control" filesystem at path
	GetPids() ([]int, error)

	// Returns statistics for Intel RDT
	GetStats() (*Stats, error)

	// Destroys the Intel RDT "resource control" filesystem
	Destroy() error

	// Returns Intel RDT "resource control" filesystem path to save in
	// a state file and to be able to restore the object later.
	GetPath() string

	// Set Intel RDT "resource control" filesystem as configured.
	Set(container *configs.Config) error
}

// This implements interface Manager
type IntelRdtManager struct {
	mu      sync.Mutex
	Cgroups *configs.Cgroup
	Path    string
}

type intelRdtData struct {
	root   string
	config *configs.Cgroup
	pid    int
}

// The absolute path to the root of the Intel RDT "resource control" filesystem.
var intelRdtRootLock sync.Mutex
var intelRdtRoot string

// Gets the root path of Intel RDT "resource control" filesystem.
func getIntelRdtRoot() (string, error) {
	intelRdtRootLock.Lock()
	defer intelRdtRootLock.Unlock()

	if intelRdtRoot != "" {
		return intelRdtRoot, nil
	}

	root, err := findIntelRdtMountpointDir()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(root); err != nil {
		return "", err
	}

	intelRdtRoot = root
	return intelRdtRoot, nil
}

func getIntelRdtData(c *configs.Cgroup, pid int) (*intelRdtData, error) {
	rootPath, err := getIntelRdtRoot()
	if err != nil {
		return nil, err
	}
	return &intelRdtData{
		root:   rootPath,
		config: c,
		pid:    pid,
	}, nil
}

func isIntelRdtMounted() bool {
	_, err := getIntelRdtRoot()
	if err != nil {
		if !IsNotFound(err) {
			return false
		}

		// If not mounted, we try to mount again:
		// mount -t rscctrl rscctrl /sys/fs/rscctrl
		if err := os.MkdirAll("/sys/fs/rscctrl", 0755); err != nil {
			return false
		}
		if err := exec.Command("mount", "-t", "rscctrl", "rscctrl", "/sys/fs/rscctrl").Run(); err != nil {
			return false
		}
	}

	return true
}

// Check if Intel RDT is enabled
func IsIntelRdtEnabled() bool {
	// 1. check if hardware and kernel support Intel RDT feature
	// "rdt" flag is set if supported
	isFlagSet, err := parseCpuInfoFile("/proc/cpuinfo")
	if err != nil {
		return false
	}

	// 2. check if Intel RDT "resource control" filesystem is mounted
	isMounted := isIntelRdtMounted()

	return isFlagSet && isMounted
}

// Applies configuration to the process with the specified pid
func (m *IntelRdtManager) Apply(pid int) (err error) {
	d, err := getIntelRdtData(m.Cgroups, pid)
	if err != nil && !IsNotFound(err) {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := d.join(m.Path)
	if err != nil {
		return err
	}

	m.Path = path
	return nil
}

// Destroys the Intel RDT "resource control" filesystem
func (m *IntelRdtManager) Destroy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.RemoveAll(m.Path); err != nil {
		return err
	}
	m.Path = ""
	return nil
}

// Returns Intel RDT "resource control" filesystem path to save in
// a state file and to be able to restore the object later.
func (m *IntelRdtManager) GetPath() string {
	m.mu.Lock()
	path := m.Path
	m.mu.Unlock()
	return path
}

// Returns statistics for Intel RDT
func (m *IntelRdtManager) GetStats() (*Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := NewStats()
	subPath := m.Path
	rootPath, err := getIntelRdtRoot()
	if err != nil {
		return nil, err
	}

	// The read-only stats in root of Intel RDT "resource control" filesystem
	path := filepath.Join(rootPath, "info")
	info, err := getIntelRdtParamString(path, "info")
	if err != nil {
		return nil, err
	}
	path = filepath.Join(rootPath, "info", "l3")
	domainToCacheId, err := getIntelRdtParamString(path, "domain_to_cache_id")
	if err != nil {
		return nil, err
	}
	maxCbmLen, err := getIntelRdtParamUint(path, "max_cbm_len")
	if err != nil {
		return nil, err
	}
	maxClosid, err := getIntelRdtParamUint(path, "max_closid")
	if err != nil {
		return nil, err
	}
	rootL3CacheSchema, err := getIntelRdtParamString(rootPath, "schemas")
	if err != nil {
		return nil, err
	}
	rootL3CacheCpus, err := getIntelRdtParamString(rootPath, "cpus")
	if err != nil {
		return nil, err
	}
	stats.IntelRdtRootStats.Info = info
	stats.IntelRdtRootStats.DomainToCacheId = domainToCacheId
	stats.IntelRdtRootStats.MaxCbmLen = maxCbmLen
	stats.IntelRdtRootStats.MaxClosid = maxClosid
	stats.IntelRdtRootStats.RootL3CacheSchema = rootL3CacheSchema
	stats.IntelRdtRootStats.RootL3CacheCpus = rootL3CacheCpus

	// The stats in "container_id" partition
	schema, err := getIntelRdtParamString(subPath, "schemas")
	if err != nil {
		return nil, err
	}
	cpus, err := getIntelRdtParamString(subPath, "cpus")
	if err != nil {
		return nil, err
	}
	stats.IntelRdtSubStats.L3CacheSchema = schema
	stats.IntelRdtSubStats.L3CacheCpus = cpus

	return stats, nil
}

// Set Intel RDT "resource control" filesystem as configured.
func (m *IntelRdtManager) Set(container *configs.Config) error {
	path := m.GetPath()

	// About L3 cache schema file:
	// The schema has allocation masks/values for L3 cache on each socket,
	// which contains L3 cache id and capacity bitmask (CBM).
	//     Format: "L3:<cache_id0>=<cbm0>;<cache_id1>=<cbm1>;..."
	// For example, on a two-socket machine, L3's schema line could be:
	//     L3:0=ff;1=c0
	// Which means L3 cache id 0's CBM is 0xff, and L3 cache id 1's CBM is 0xc0.
	//
	// About L3 cache CBM validity:
	// The valid L3 cache CBM is a *contiguous bits set* and number of
	// bits that can be set is less than the max bit. The max bits in the
	// CBM is varied among supported Intel Xeon platforms. In Intel RDT
	// "resource control" filesystem layout, the CBM in a "partition"
	// should be a subset of the CBM in root. Kernel will check if it is
	// valid when writing.
	// e.g., 0xfffff in root indicates the max bits of CBM is 20 bits,
	// which mapping to entire L3 cache capacity. Some valid CBM values
	// to set in a "partition": 0xf, 0xf0, 0x3ff, 0x1f00 and etc.
	l3CacheSchema := container.Cgroups.Resources.IntelRdtL3CacheSchema
	if l3CacheSchema != "" {
		if err := writeFile(path, "schemas", l3CacheSchema); err != nil {
			return err
		}
	}

	// The bitmask of the CPUs that are bound to the schema
	l3CacheCpus := container.Cgroups.Resources.IntelRdtL3CacheCpus
	if l3CacheCpus != "" {
		if err := writeFile(path, "cpus", l3CacheCpus); err != nil {
			return err
		}
	}

	return nil
}

// Returns the PIDs inside Intel RDT "resource control" filesystem at path
func (m *IntelRdtManager) GetPids() ([]int, error) {
	return readTasksFile(m.GetPath())
}

func (raw *intelRdtData) join(name string) (string, error) {
	path := filepath.Join(raw.root, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	if err := WriteIntelRdtTasks(path, raw.pid); err != nil {
		return "", err
	}
	return path, nil
}

type NotFoundError struct {
	ResourceControl string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("mountpoint for %s not found", e.ResourceControl)
}

func NewNotFoundError(res string) error {
	return &NotFoundError{
		ResourceControl: res,
	}
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*NotFoundError)
	return ok
}
