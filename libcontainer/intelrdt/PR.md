Proposal: Intel RDT/CAT support in runc/libcontainer  
https://github.com/opencontainers/runc/issues/433

```
The descriptions of Intel RDT/CAT features, user cases and Linux kernel interface are heavily based
on the Intel RDT documentation of the Linux kernel patches:
https://lkml.org/lkml/2016/7/12/747
https://lkml.org/lkml/2016/7/12/764

Thanks to the authors of the kernel patches:
* Vikas Shivappa <vikas.shivappa@linux.intel.com>
* Fenghua Yu <fenghua.yu@intel.com>
* Tony Luck <tony.luck@intel.com>
```

## What is Intel RDT and CAT:

Intel Cache Allocation Technology (CAT) is a sub-feature of Resource Director Technology (RDT). Currently L3 Cache is the only resource that is supported in RDT.

Cache Allocation Technology offers the capability of L3 cache Quality of Service (QoS). It provides a way for the Software (OS/VMM/Container) to restrict cache allocation to a defined 'subset' of cache which may be overlapping with other 'subsets'. This feature is used when allocating a line in cache i.e. when pulling new data into the cache. The programming of the h/w is done via PQR MSRs.

The different cache subsets are identified by CLOS (class of service) identifier and each CLOS has a CBM (cache bit mask). The CBM is a contiguous set of bits which defines the amount of cache resource that is available for each 'subset'.

More information can be found in the section 17.17 of [Intel Software Developer Manual] [1] and [Intel RDT/CAT introduction] [2].
[1]: http://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-manual-325462.pdf
[2]: https://software.intel.com/en-us/articles/introduction-to-cache-allocation-technology


## Supported Intel Xeon CPU SKUs:

* Intel(R) Xeon(R) processor E5 v4 and newer generations
* Intel(R) Xeon(R) processor D
* Intel(R) Xeon(R) processor E5 v3 (limited support)

To check if cache allocation was enabled:

$ cat /proc/cpuinfo

Check if output have 'rdt' and 'cat_l3' flags.


## Why is Cache Allocation needed:

Cache Allocation Technology is useful in managing large computer server systems with large size L3 cache, in the cloud and container context. Examples may be large servers running instances of webservers or database servers. In such complex systems, these subsets can be used for more careful placing of the available cache resources by a centralized root accessible interface.

The architecture also allows dynamically changing these subsets during runtime to further optimize the performance of the higher priority application with minimal degradation to the low priority app. Additionally, resources can be rebalanced for system throughput benefit.


## User cases for container:

Figure 1:
![cat_case](https://cloud.githubusercontent.com/assets/16153452/11753748/fe4c73ce-a081-11e5-90e4-f6070e0d3142.png)

Note: Figure 1 is fetched from section 17.17 of [Intel Software Developer Manual] [1].
[1]: http://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-manual-325462.pdf
Currently the Last Level Cache (LLC) in Intel Xeon platforms is L3 cache. So LLC == L3 cache here.


### Noisy neighbor issue:
A typical use case is to solve the noisy neighbor issue in container environment. For example, when a streaming application which running in a container is constantly copying data and accessing linear space larger than L3 cache, and hence evicting a large amount of cache which could have otherwise been used by a higher priority computing application which running in another container.

Using the cache allocation feature, the 'noisy neighbors' container which running the streaming application can be confined to use a smaller cache, and the higher priority application be awarded a larger amount of L3 cache space.

### L3 cache QoS:
Another key user scenario is in large-scale container clusters context. A central scheduler or orchestrator would control resource allocations to a set of containers. Docker and runc can make use of libcontainer to manage resources. They could benefit from Intel RDT cache allocation feature for new resource constraints. We could define different cache subsets strategies through setting different CLOS/CBM in containers' runtime configuration. As a result, we could achieve fine-grained L3 cache QoS (quality of service) among containers.


## Linux kernel interface for Intel RDT/CAT:

In Linux kernel, Intel RDT/CAT will be supported with kernel config CONFIG_INTEL_RDT.

Originally, the kernel interface for Intel RDT/CAT is `intel_rdt` cgroup, but the cgroup solution is rejected by kernel cgroup maintainer for some reasons, such as incompatibility with cgroup hierarchy, limitations for some corner cases and etc.

Currently, a new kernel interface is defined and exposed via "resource control" filesystem, which is a "cgroup-like" interface. The new design aligns better with the hardware capabilities provided, and addresses the issues in cgroup based interface. Comparing with cgroups, the interface has similar process management lifecycle and interfaces in a container. But unlike cgroups' hierarchy, it has single level filesystem layout.

### Intel RDT "resource control" filesystem hierarchy:
```
mount -t rscctrl rscctrl /sys/fs/rscctrl
tree /sys/fs/rscctrl
/sys/fs/rscctrl
|-- cpus
|-- info
|   |-- info
|   |-- l3
|       |-- domain_to_cache_id
|       |-- max_cbm_len
|       |-- max_closid
|-- schemas
|-- tasks
|-- <container_id>
    |-- cpus
    |-- schemas
    |-- tasks
```

The file `tasks` has all task ids belonging to the partition "container_id". The task ids in the file will be added or removed among partitions. A task id only stays in one directory at the same time.

The file `schemas` has allocation bitmasks/values for L3 cache on each socket, which contains L3 cache id and capacity bitmask (CBM).
```
	Format: "L3:<cache_id0>=<cbm0>;<cache_id1>=<cbm1>;..."
```
For example, on a two-socket machine, L3's schema line could be `L3:0=ff;1=c0` which means L3 cache id 0's CBM is 0xff, and L3 cache id 1's CBM is 0xc0.

The valid L3 cache CBM is a *contiguous bits set* and number of bits that can be set is less than the max bit. The max bits in the CBM is varied among supported Intel Xeon platforms. In Intel RDT "resource control" filesystem layout, the CBM in a "partition" should be a subset of the CBM in root. Kernel will check if it is valid when writing. e.g., 0xfffff in root indicates the max bits of CBM is 20 bits, which mapping to entire L3 cache capacity. Some valid CBM values to set in a "partition": 0xf, 0xf0, 0x3ff, 0x1f00 and etc.

The file `cpus` has a cpu bitmask that specifies the CPUs that are bound to the schemas. Any tasks scheduled on the cpus will use the schemas.

For more information about Intel RDT/CAT kernel interface:  
https://lkml.org/lkml/2016/7/12/764

### An example for runc:
```
There are two L3 caches in the two-socket machine, the default CBM is 0xfffff and the max CBM length
is 20 bits. This configuration assigns 4/5 of L3 cache id 0 and the whole L3 cache id 1 for the container:

"linux": {
	"resources": {
		"intelRdt": {
			"l3CacheSchema": "L3:0=ffff0;1=fffff",
			"L3CacheCpus": "00000000,00000000,00000000,00000000,00000000,00000000"
		}
	}
}
```


## Proposal and design

### Intel RDT/CAT support in runtime-spec:
This is the prerequisite of this proposal. A new pull request will be open in *github.com/opencontainers/runtime-spec* soon.
* Add Intel RDT/CAT L3 cache resources in Linux-specific configuration to support `config.json`.

### Intel RDT/CAT support in runc/libcontainer
This proposal is mainly focused on Intel RDT/CAT infrastructure in libcontainer:
* Add **`package intelrdt`** as a new infrastructure in libcontainer. It implements `Manager interface` which is similar to cgroup manager framework:
  * *Apply()*
  * *Set()*
  * *GetStats()*
  * *GetPids()*
  * *GetPath()*
  * *Destroy()*
* Add **`intelRdtManager`** in `linuxContainer struct`, and invoke Intel RDT/CAT operations in process management (`initProcess`, `setnsProcess`) functions:
  * *Start()*
  * *Set()*
  * *Stats()*
* Add hook function to configure a `LinuxFactory` to return containers which could create and manage Intel RDT/CAT L3 cache resources:
  * *Create()*
  * *Load()*
* Add Intel RDT/CAT entries in libcontainer/configs.
* Add runtime-spec configuration handler in *CreateLibcontainerConfig()*.
* Add Intel RDT/CAT stats metrics in libcontainer/intelrdt.
* Add Intel RDT/CAT unit test cases in libcontainer/intelrdt.
* Add Intel RDT/CAT integration test cases in libcontainer/integration.
* Add runc documentations for Intel RDT/CAT.

### TODO list:
#### Intel RDT/CAT support in Docker
When Intel RDT/CAT is ready in libcontainer, Docker could naturally make use of libcontainer to support L3 cache allocation for container resource management. Some potential work to do in Docker:
* Add `docker run` options to support Intel RDT/CAT.
* Add Docker configuration file options to support Intel RDT/CAT.
* Add `docker client/daemon` APIs to support Intel RDT/CAT.
* Add Intel RDT/CAT functions in `docker engine` and `containerd`.
* Add Intel RDT/CAT L3 cache metrics in `docker stats`.
* Add Docker documentations for Intel RDT/CAT.
* Add easy-to-use interface in Docker daemon swarm mode.

#### Intel RDT/CDP support in runc
As a specialized extension of CAT, Code and Data Prioritization (CDP) enables separate control over code and data placement in the L3 cache. Certain specialized types of workloads may benefit with increased runtime determinism, enabling greater predictability in application performance.

The Linux kernel CDP patch is part of CAT patch series. We can also add the functionality in runc.


## Obsolete design which based on cgroup interface

The following content is kept only for reference. The original design based on kernel **`cgroup`** interface will be obsolete for kernel cgroup interface patch is rejected.
```
### L3 cache QoS through cgroup:
Another key user scenario is in large-scale container clusters context. A central scheduler or orchestrator would control resource allocations to a set of containers. In today's resource management, cgroups are widely used and a significant amount of plumbing in user space is already done to perform tasks like allocating and configuring resources dynamically and statically.

Docker and runc are using cgroups interface via libcontainer to manage resources. They could benefit from cache allocation feature for cgroup interface is an easily adaptable interface for L3 cache allocation. We could define different cache subsets strategies through setting different CLOS/CBM in containers' runtime configuration. As a result, we could achieve fine-grained L3 cache QoS (quality of service) among containers.

## Linux Kernel intel_rdt cgroup interface:

In Linux kernel 4.6 (or later), new cgroup subsystem 'intel_rdt' *will be* added soon with kernel config CONFIG_INTEL_RDT.
The latest [Intel Cache Allocation Technology (CAT) kernel patch] [2]:
[2]: https://lkml.org/lkml/2015/10/2/72
https://lkml.org/lkml/2015/10/2/72

The different L3 cache subsets are identified by CLOS identifier (class of service) and each CLOS has a CBM (cache bit mask). The CBM is a contiguous set of bits which defines the amount of cache resource that is available for each 'subset'.

The max CBM, which mapping to entire L3 cache, is indicated by *intel_rdt.l3cbm* in the root node. The value is varied among different supported Intel platforms (for example, intel_rdt.l3_cbm == 0xfffff means the max CBM is 20 bits). The *intel_rdt.l3_cmb* in any child cgroup is inherited from parent cgroup by default, and it can be changed by user later.

Say if the L3 cache is 55 MB and max CBM is 20 bits. This assigns 11 MB (1/5) of L3 cache to group1 and group2 which is exclusive between them.

$ mount -t cgroup -ointel_rdt intel_rdt /sys/fs/cgroup/intel_rdt
$ cd /sys/fs/cgroup/intel_rdt
$ mkdir group1
$ mkdir group2
$ cd group1
$ /bin/echo 0xf > intel_rdt.l3_cbm
$ cd group2
$ /bin/echo 0xf0 > intel_rdt.l3_cbm

Assign tasks to the group1 and group2:

$ /bin/echo PID1 > tasks
$ /bin/echo PID2 > tasks

Linux kernel cgroup infrastructure also supports mounting cpuset and intel_rdt cgroup subsystems together. We can configure L3 cache allocation to align CPU affinity per-cores or per-socket.

Intel_rdt cgroup has zero or minimal overhead in hot path on following unsupported cases:
* Linux kernel patch doesn't exist on any non-intel platforms.
* On Intel platforms, this could not exist by default unless INTEL_RDT is enabled.
* Remains a no-op when INTEL_RDT is enabled but Intel hardware does not support the feature.

## intel_rdt cgroup support in github.com/opencontainers/specs

This is the prerequisite of this proposal. I will open a new issue or pull request in *github.com/opencontainers/specs* soon.
* Add intel_rdt cgroup resources in Linux-specific runtime configuration to support ```runtime.json```.

## intel_rdt cgroup support in runc/libcontainer

This proposal is mainly focused on intel_rdt cgroup infrastructure in libcontainer:

* Add *package IntelRdtGroup* to implement new subsystem interface in libcontainer/cgroups/fs:
  * *Name()*
  * *GetStats()*
  * *Remove()*
  * *Apply()*
  * *Set()*
* Add *IntelRdtGroup* in cgroup subsystemSet in libcontainer/cgroups/fs.
* Add intel_rdt cgroup unit tests in libcontainer/cgroups/fs.
* Add intel_rdt cgroup stats metrics in libcontainer/cgroup.
* Add systemd cgroup specific functions in libcontainer/cgroups/systemd.
  * Add *IntelRdtGroup* in cgroup subsystemSet in libcontainer/cgroups/systemd.
  * Add *joinIntelRdt()* function in *Apply()*
* Add intel_rdt cgroup entries in libcontainer/configs.
* Add intel_rdt libcontainer integration tests in libcontainer/integration.
* Add runc documentations for intel_rdt cgroup.

## intel_rdt cgroup support in Docker (TODO)
When intel_rdt cgroup is ready in libcontainer, naturally, Docker could make use of libcontainer as native execution driver to support L3 cache allocation for container resource management.

Some potential work to do in Docker in future:
* Add *docker run* options to support intel_rdt.
* Add *docker client/daemon* APIs to support intel_rdt.
* Add intel_rdt functions in *docker execution driver*.
* Add intel_rdt cgroup metrics in *docker stats*.
* Add Docker documentations for intel_rdt cgroup.
```
