---
title: HAMi-WebUI Refactoring and Multi-Cluster Capability
authors:
- "@Shenhan11"
reviewers:
- ""
approvers:
- ""
creation-date: 2025-02-09

---

# HAMi-WebUI Refactoring and Multi-Cluster Capability

## Summary

<!--
This section is for user-facing documentation (e.g. release notes or roadmap). A good Summary has at least one complete paragraph.
-->

The current HAMi-WebUI has multiple issues in build system, front-end architecture, UI consistency, maintainability, and extensibility, making it difficult to improve experience and deliver new capabilities on the existing architecture. At the same time, monitoring and observability suffer from incomplete metrics and unclear presentation, affecting users’ understanding of GPU resources and task status. In addition, with single-cluster monitoring and management largely in place, demand is growing for cross-cluster visibility of GPU resources, tasks, and node status; a single-cluster WebUI cannot meet unified observation and control in multi-cluster scenarios.

This proposal clarifies refactoring and evolution goals, improves completeness and clarity of metric presentation, and adds multi-cluster capability: supporting presentation of nodes, GPU resources, tasks, and related data for multiple clusters in one platform, improving HAMi’s usability and operational efficiency in multi-cluster environments.

## Motivation

<!--
This section states the motivation, goals, and non-goals of this KEP. It explains why the change matters and what users gain.
-->

The current HAMi-WebUI has architecture and implementation issues: dual build systems, inconsistent environment variables, route/sidebar null-safety risks, mixed UI frameworks, and lack of TypeScript, leading to high maintenance cost and limited stability and extensibility. On monitoring and observability, there are missing key metrics (including GPU, CPU, memory, and task-related metrics), unclear presentation (e.g. high data density, lack of charts and visualizations, key information buried in tables), and insufficient interaction (e.g. no way to jump from task details to the node and GPU where it runs), which affect observation and troubleshooting of resource and task status.

Also, the current WebUI dosen't keep up with the development of HAMi (e.g. The `DeviceInfo` structure has never been updated to reflect the latest changes in HAMi since last year), which makes it difficult to maintain and update.

In addition, as more users deploy HAMi across multiple clusters, demand for unified multi-cluster observation is growing; a single-cluster WebUI cannot meet that need.

Therefore, "addressing existing WebUI issues", "improving metric completeness and clarity", and "adding multi-cluster capability" are brought together into one evolution direction: first address build unification and high-priority stability, then complete metrics and interaction, and on that basis extend to multi-cluster observation—improving HAMi-WebUI's long-term value through refactoring and new capabilities.

### Goals

<!--
List the concrete goals of this KEP. What should be achieved? How is success measured?
-->

1. **Unify build system and fix high-priority stability issues**: Single build system (Vite only), consistent env and port documentation, fix sidebar `route.matched[0]` null-safety and similar stability issues.
2. **Promote the release process**: Follow the version and release process of HAMi to ensure the compatibility of the WebUI with HAMi (e.g. data structure and metrics compatibility).
3. **Improve completeness and clarity of metric presentation**:
   - Add missing key metrics (including GPU, CPU, memory, and task-related metrics).
   - Improve presentation: add charts and visualizations (e.g. time series, heatmaps, status distribution), optimize list density and highlight key information, and provide clearer data hierarchy and interaction.
   - Improve task-detail interaction: support jumping from a task to its node and GPU detail pages, and viewing related task lists from node/GPU details.
4. **Add multi-cluster capability**:
   - Support connecting to and observing multiple clusters in the WebUI.
   - Support viewing nodes, GPUs, tasks, and related resources by cluster; support multi-cluster aggregate views.

### Non-Goals

<!--
Out of scope for this KEP; stating this helps focus discussion and progress.
-->

1. **No task or resource creation via WebUI**: The WebUI is for monitoring and observation only; it does not provide create, edit, or delete operations.

## Proposal

<!--
This section describes the proposal so reviewers understand the gist and expected outcome. Detailed design is in "Design Details".
-->

### User Stories (Optional)

**Story 1: Complete metric presentation**
As a user, I only see partial basic metrics on the resource overview and node/GPU detail pages. Due to missing CPU/memory-related metrics and task distribution across nodes/GPUs, I expect to see more complete metrics and quickly understand resource status through charts and visualizations.

**Story 2: Clear information presentation**
As a user, when I view node or GPU lists, key information is buried in dense tables. Due to high data density and lack of visualization, I expect key information to be clearly highlighted and to use charts (e.g. time series, heatmaps, status distribution) to quickly understand overall status.

**Story 3: Multi-cluster Dashboard summary**
As a user, I deploy HAMi in multiple Kubernetes clusters and have to open each cluster’s WebUI to see its overall status. Because this is fragmented and inefficient, I expect a multi-cluster Dashboard that shows each cluster’s summary (e.g. GPU total and utilization, node count, task count) in one page and allows switching the currently viewed cluster to see that cluster’s overview summary and quickly compare status across clusters.

**Story 4: Cluster list and detail view**
As a user, I manage many clusters and today need the CLI or multiple WebUIs to inspect each cluster. For easier management, I expect a cluster list page that shows basic info (name, labels, version, node count, status) per cluster and allows clicking into a cluster’s detail page to see overview, nodes, compute, and tasks for that cluster.

### Notes/Constraints/Caveats (Optional)

The current WebUI has issues in build system, front-end architecture, UI consistency, maintainability, and metric presentation. This KEP brings these into scope for refactoring and addresses them by priority, with high priority (stability, build unification, metric completeness) addressed first.

Until the new WebUI is implemented and replaces the current interface, the existing WebUI is treated as a **legacy WebUI** in maintenance mode: only important bugs will be fixed, and no new features will be added on top of the legacy UI.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Scope creep from doing refactor, metric improvement, and multi-cluster together | Define phases: fix high-priority issues (stability, build unification, metric completeness) and unify build first, then improve presentation clarity, then add multi-cluster access and presentation, and finally aggregate views. |
| Insufficient metric data sources | Roll out metric improvements in phases: first surface existing backend metrics not yet shown in the UI; coordinate with backend for new metrics and expose them only when data is available. |
| Impact on existing single-cluster users | In single-cluster setups the cluster list shows one cluster; clicking it shows that cluster’s data. Interaction matches multi-cluster; only the amount of list data differs. Metric changes are additive; do not remove existing presentation to avoid breaking current users. |

## Design Details

<!--
This section contains the concrete design details for implementation and review.
-->

### Refactoring priority

- **High**: Unify build (Vite only), fix high-priority stability (e.g. sidebar null-safety), add missing key metrics (e.g. GPU metrics, task history trends), improve presentation clarity (charts, density, highlighting).
- **Medium**: Unify env vars to `VITE_*`, converge UI framework, remove unused components and pages.
- **Low**: Introduce TypeScript.

### Feature outline (multi-cluster and metrics)

- **New multi-cluster Dashboard**
  - A dedicated Dashboard page showing per-cluster summary in cards or list (e.g. cluster name, total GPU and utilization, node count, task count, status).
  - Support switching the currently viewed cluster on the Dashboard; after switching, the page **shows only that cluster’s overview summary** (charts and stats update accordingly). The Dashboard does not provide full cluster detail; to see a cluster’s nodes, compute, tasks, etc., the user **clicks the cluster in the cluster list to open that cluster’s detail page**.

- **New cluster list and detail pages**
  - **Cluster list**: Shows basic info (name, labels, version, node count) for all clusters as the entry to each cluster’s detail.
  - **Cluster detail**: Presents overview, nodes, compute, and tasks for that cluster; structure matches current single-cluster pages, with entry from the cluster list.

- **New or extended key metrics (examples)**
  - **Presentation**: Use cards, summary areas, or similar to highlight key metrics so they are not buried in lists.
  - **CPU/memory-related**: Node CPU/memory usage and quota, per-task CPU/memory usage and allocation, etc.
  - **Task-related**: Task distribution across nodes/GPUs, per-task GPU memory/compute and CPU/memory usage and quota, task history trends, resource quota utilization, etc.
  - **Cluster level**: GPU total and utilization, node count, active task count (concrete metrics to be exposed as backend supports them).

- **New Controller to maintain multi-cluster resources**
  - **Cluster CRD**: A Cluster CRD to store cluster-specific metadata (e.g. name, labels, version, node count, access info).
  - **Cluster controller**: Automatically discover and maintain Cluster CRDs for all clusters and configure multiple clusters monitoring.

- **Interaction improvements**
  - In task detail, show the node and GPU where the task runs and provide one-click jump to node/GPU detail.
  - In node/GPU detail, support viewing the list of tasks currently running there for two-way linkage between task view and resource view.

### Configuration and compatibility

- **Single-cluster users**: The cluster list shows one cluster; the Dashboard shows that cluster’s overview summary. Interaction is the same as multi-cluster with fewer list entries.
- **Multi-cluster users**: Cluster list and Dashboard show all clusters (from config or backend discovery). To see full cluster detail (overview, nodes, compute, tasks), the user clicks a cluster in the cluster list to open its detail page. Switching the current cluster on the Dashboard only shows that cluster’s overview summary and does not navigate to cluster detail.

## Plan

1. Regression tests for existing single-cluster flow and key pages (resource overview, nodes, GPU, tasks) remain passing.
2. Metric improvements: Verify correctness of new metrics, chart rendering, and effectiveness of key-information highlighting.
3. Multi-cluster: Add tests for “data correct after switching current cluster on Dashboard” and “single-cluster scenario data meets expectations”.
4. Build: After unifying on Vite, CI runs only Vite build.

## Alternatives Considered

- **Only fix bugs in current codebase, no refactor**: Would improve stability somewhat but would not unify build and architecture or extend to multi-cluster at low cost; therefore the “refactor + multi-cluster” evolution is adopted.
- **Multi-cluster via multiple browser tabs, one per cluster**: Poor UX and no aggregate view; supporting cluster switching and optional aggregation inside the WebUI better fits operator workflows.
