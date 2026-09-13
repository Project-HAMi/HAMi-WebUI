# GPU workload scheduling information

The Workloads page uses one table for HAMi-allocated containers and containers
with recognized GPU requests, including those without a HAMi allocation record.
Pending scheduling, starting,
running, and abnormal workloads share the same identity column, filters, sorting,
and pagination. Each row is a Pod/container pair. Containers from the same Pod
share its scheduling diagnosis because Kubernetes schedules them together.

Open a workload without an allocation record to inspect its requests, scheduling feedback, and
recorded events. The view is read-only. It does not submit a scheduling attempt,
invoke HAMi's filter endpoint, or modify the Pod.

The scheduling information opens with a conclusion: the stage, how long the Pod
has waited, and each unmet condition the scheduler reported. Suggested checks
follow. GPU requests are shown per container. Scheduling constraints such as
node selectors, tolerations, affinity, topology spread, and HAMi device
annotations are listed with their original definitions one click away. Recorded
events stay collapsed at the end.

## Understand the result

- A Pod without a node is pending scheduling. A scheduling gate means a controller
  deliberately holds it; this does not establish a GPU capacity shortage.
- A Pod with a node has been assigned, even if its phase is still `Pending` while
  an image is being prepared. Image-pull and application failures belong to the
  workload's startup or runtime status.
- The latest cached Pod condition supplies the scheduling summary. Events are
  timestamped records; an old failure does not override a newly assigned node.
- `schedulerName` identifies the configured scheduler profile. It does not prove
  HAMi rejected the Pod: CPU, host memory, taints, affinity, and PVC checks can
  fail before the HAMi extender runs.
- HAMi's default `forceOverwriteDefaultScheduler` setting can replace an explicit
  `default-scheduler` with its configured scheduler. The UI shows the stored Pod
  after admission. If the name is retained, check whether that scheduler actually
  integrates with HAMi; its name alone proves neither integration nor failure.
- Kubernetes `Insufficient <GPU resource name>` is reported as a Kubernetes
  resource request failure, separately from HAMi's per-GPU compute and memory
  feedback. A configured resource name missing from node capacity can produce
  this message without establishing that physical GPU memory is exhausted.
- HAMi memory feedback describes schedulable capacity on matching devices. It is
  different from physical memory usage measured by telemetry. Shared-slot limits,
  device constraints, and namespace quotas are also distinct from free capacity.
- A Pod condition's transition time is not the time of its most recent failed
  scheduling attempt.
- WebUI recognizes HAMi's device feedback, including custom filter rules and
  scheduling mode mismatches. It also recognizes common Kubernetes reasons:
  insufficient CPU or host memory, unavailable GPU resources, taints, node
  affinity, cordoned nodes, Pod affinity and anti-affinity, the per-node Pod
  limit, host port conflicts, and volume problems.
- When the scheduler reports a reason that WebUI does not recognize, the diagnosis
  points to the original records. It suggests checking the scheduler only when no
  scheduling feedback exists.

All rows use three compact values: GPU count, compute in card equivalents, and
total memory in GiB. The workload status provides the lifecycle context without
extra request/allocation labels. Quantities without allocation records come from the container's
explicit requests; known per-GPU amounts are multiplied by the GPU count.
Missing defaults, device-dependent memory percentages, zero absolute memory
requests, and unknown resource units display `--` rather than a guessed total.
Original per-GPU requests remain available in the scheduling information.
Requests do not become allocations, and rankings continue to describe allocation records.
Existing HAMi reservations before node binding remain in the allocation
accounting and metrics without appearing as a second row for the same container.
A reservation that no pending request represents, for example because its
resource names are not configured for discovery, stays in the list as an
allocation row.

The status control above the table shows how many rows each status holds. The
counts apply the node, GPU, and name filters but not the status itself, so
switching between statuses does not change them. The selected status, filters,
and page are kept in the page address, so returning from a workload restores
them.

The list orders abnormal states first. A state that cannot be confirmed, such as
after a node is lost, is shown and filtered as abnormal. Pending scheduling
follows with the longest wait first, and its status explanation includes the
wait time. Starting, terminating, running, and completed states come next.
Otherwise, namespace, Pod, container identity, and UID keep the order stable.
Name filters search both Pod and container names. Node and GPU filters match actual placement, so they exclude
unbound requests even if a request names a preferred node or GPU.

GPU containers without HAMi allocation records remain visible after node
assignment. Their status comes from the observed container state, not inferred
allocation success. Diagnosis states that no HAMi record was found and suggests
checking scheduler integration and the device plugin when HAMi is expected.
Real allocation records take precedence for the same Pod/container identity.
These additional rows never enter allocation rankings or GPU-specific filters
without a device allocation. Node filters use the observed node assignment.
GPU init containers are labeled; ordinary CPU containers remain outside this view.
HAMi can also allocate GPUs to init and sidecar containers. Those allocations are
not yet part of the allocation records, so the list shows such a container as a
request and its diagnosis can report that no allocation record was found.

The list covers workloads that currently wait for, hold, or fail to use a GPU.
Finished Pods, whose phase is Succeeded or Failed, are not listed: they hold no
HAMi allocation and no longer wait for scheduling, and their retention depends on
cluster cleanup. A container without an allocation record that has completed
normally, such as a finished GPU init container, is not listed either. A
completed container with an allocation record stays visible, because HAMi keeps
a Pod's allocation until the Pod ends.

## Configure request discovery

By default workload discovery recognizes positive requests for these exact keys:

```text
nvidia.com/gpu
nvidia.com/gpucores
nvidia.com/gpumem
nvidia.com/gpumem-percentage
```

For custom HAMi resource names or another accelerator, configure the exact keys:

```yaml
# Helm values. A nonempty list replaces the defaults.
scheduling:
  resourceNames:
    - example.com/gpu
    - example.com/gpu-memory
```

The corresponding standalone backend setting is `scheduling.resource_names`.
Names must be qualified Kubernetes resource names. At most 64 unique keys are
accepted. Include all relevant count, compute, and memory keys. Keys are not
discovered by looking for the substring `gpu`.

Default NVIDIA resource quantities have known display units. Custom keys keep
their original names and quantities; their values are not guessed to mean a
percentage or bytes. Requests from regular and init containers remain separate.
The UI does not sum them into a purported Kubernetes effective request.
The NVIDIA MiB mapping assumes HAMi's default memory factor of one; this WebUI
does not currently read a customized HAMi memory factor.

Native DRA claims, unknown resource keys, and submissions rejected before a Pod
exists are outside this discovery path. No absence of a row or event proves the
absence of a scheduling issue.

## Events and API-server load

The unified list combines the existing allocation cache with an index on the
existing Pod informer, then filters, sorts, and paginates on the backend. It adds no Pod watch and
does not query Events for rows, hover, filtering, rankings, or pagination.
Opening a diagnosis or explicitly refreshing it requests events for the selected
Namespace and Pod UID. Closing it stops further requests. Recreating a Pod with
the same name does not reuse the old Pod's events.

The Helm reader role adds only `list` on core `events`. Installations managing
their own RBAC must grant that permission to the WebUI service account. Without
it, Pod information remains available and the event section reports the missing
permission. No event watch, Pod log, or Secret permissions are needed.

Event queries are bounded and shared within one backend instance. They use a
short cache, combine concurrent requests for the same UID, and limit both query
rate and retained data. Multiple backend replicas each have their own budget.
The current limits are two concurrent queries, one HTTP request per second with
a burst of two, and a three-second deadline. Each query reads at most three pages
of 200 events, with a 2 MiB response-body limit per page before decoding. It
retains at most 50 records and 4 KiB per message. The cache expires after 15 seconds
(two seconds for unavailable results), with at most 128 Pod UIDs and 8 MiB of
serialized event payload; this is not a bound on the backend's total heap.
Limits and truncated results are disclosed; retained records are not a complete
audit history. Kubernetes events also expire according to the API server's
configured event retention.

Use the raw records when reporting an issue, together with the actual HAMi
version, selected scheduler, and resource request. The diagnosis reports known
feedback; it does not recompute all placement constraints or promise an exact
resource deficit.
