# NVIDIA GPU

HAMi-WebUI reads NVIDIA GPUs from the registration HAMi's device plugin writes
on each node, `hami.io/node-nvidia-register`, and their allocations from the
Pod annotations HAMi's scheduler writes.

## Split modes

A GPU is registered in one operating mode, and the accelerator pages show it.

| Registered mode | Shown as | How HAMi divides the GPU |
| --- | --- | --- |
| `hami-core` | HAMi-core mode | Workloads share the GPU with memory and compute quotas |
| `mig` | MIG | Workloads get MIG instances of fixed profiles and placements |
| `mps` | HAMi-core mode | The plugin registers the mode, but through HAMi v2.10 only `mig` changes how it allocates, so these GPUs run HAMi-core |
| none | HAMi-core mode | Registrations older than device modes |

## Allocations

Each allocated GPU is reported as HAMi-core, MIG or unknown. WebUI names a mode
only when HAMi's records agree on it:

- **MIG**: the scheduler reserved the instance in `hami.io/vgpu-mig-allocations`
  for that container slot and GPU, and the GPU is registered in MIG mode. The
  reservation names the profile and its placement. Allocations recorded before
  HAMi v2.10 carry the template and slot in the device UUID (`GPU-…[1-2]`); they
  show as MIG without a profile.
- **HAMi-core**: the GPU is registered in HAMi-core mode and holds no MIG
  reservation.
- **Unknown**: anything else, such as a MIG GPU without its reservation, a
  reservation for another GPU, a node whose registration cannot be read, or a
  reservation annotation that HAMi's own decoder would reject. Memory and
  compute still come from the allocation.

## Split layout

The accelerator and workload pages draw how a GPU is divided right now. A MIG
GPU is drawn in the placement space its plugin registered in `migProfiles`: a
dashed slice can still take an instance of some profile, a hatched one fits
none, and the summary lists the profiles that still fit. A HAMi-core GPU shows
its memory and compute quotas instead, since nothing is carved out.

The layout is built from the workload list, so it has that list's limits:

- at most 100 containers per device are drawn;
- allocations of init containers are not listed;
- HAMi v2.10.0 records the container slot of an allocation incorrectly when a
  Pod has init or sidecar containers that request no GPU
  ([HAMi#2723](https://github.com/Project-HAMi/HAMi/pull/2723), fixed after
  that release), so such an allocation can appear under another container.
