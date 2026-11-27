export interface ApiErrorBody {
  code?: number;
  reason?: string;
  message?: string;
  metadata?: Record<string, string>;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface FieldState {
  state: 'known' | 'unknown' | 'stale' | 'conflict' | 'not_applicable' | string;
  reason: string;
  params: Record<string, string>;
  source: string;
  scope: string;
  observedAt?: string | null;
}

export interface SourceState {
  source: string;
  state: string;
  reason: string;
  params: Record<string, string>;
}

export interface FieldCoverage {
  entity: string;
  known: number;
  stale: number;
  unknown: number;
  conflict: number;
  notApplicable: number;
  reasons: Record<string, number>;
}

export interface Coverage {
  entity: string;
  observed: number;
  total?: number;
  complete: boolean;
  sources: SourceState[];
  fields: Record<string, FieldCoverage>;
}

export interface InventoryNode {
  name: string;
  uid: string;
  ready?: boolean;
  schedulable?: boolean;
  models: string[];
  physicalDevices?: number;
}

export interface InventoryDevice {
  node: string;
  provider: string;
  deviceId: string;
  model: string;
  healthy?: boolean;
  utilizationRatio?: number;
  memoryUsageRatio?: number;
}

export interface InventoryResourceSummary {
  computeAllocatedRatio?: number;
  computeCapacityRatio?: number;
  memoryAllocatedBytes?: number;
  memoryCapacityBytes?: number;
  utilizationRatio?: number;
  memoryUsedBytes?: number;
  memoryPhysicalBytes?: number;
}

export interface OverviewInventoryReply {
  snapshotAt: string;
  nodes: InventoryNode[];
  devices: InventoryDevice[];
  nodeCoverage: Coverage;
  deviceCoverage: Coverage;
  summary?: InventoryResourceSummary;
}

export type TrendScope = 'cluster' | 'node' | 'device' | 'container';

export interface TelemetryTrendsQuery {
  scope: TrendScope;
  node?: string;
  nodeUid?: string;
  deviceId?: string;
  namespace?: string;
  pod?: string;
  container?: string;
  podUid?: string;
  start?: string;
  end?: string;
  step?: string;
}

export interface TrendPoint {
  at: string;
  value?: number;
}

export interface TrendSeries {
  metric: string;
  unit: string;
  state: FieldState;
  points: TrendPoint[];
}

export interface TelemetryTrendsReply {
  scope: TrendScope | string;
  start: string;
  end: string;
  stepSeconds: string | number;
  series: TrendSeries[];
}

export interface ListNodesQuery {
  ip?: string;
  type?: string;
  isSchedulable?: boolean | string;
}

export interface NodeItem {
  ip: string;
  isSchedulable?: boolean;
  isReady?: boolean;
  type: string[];
  vgpuUsed?: number;
  vgpuTotal?: number;
  coreUsed?: number;
  coreTotal?: number;
  memoryUsed?: number;
  memoryTotal?: number;
  uid: string;
  name: string;
  cardCnt?: number;
  osImage: string;
  operatingSystem: string;
  kernelVersion: string;
  containerRuntimeVersion: string;
  kubeletVersion: string;
  kubeProxyVersion: string;
  architecture: string;
  creationTimestamp: string;
  coreUsedKnown?: boolean;
  inventoryCoverage?: Coverage;
  coreAllocationPercent?: number;
  memoryAllocationPercent?: number;
  computeUtilizationPercent?: number;
  memoryUsagePercent?: number;
  providers?: string[];
}

export interface NodesReply {
  list: NodeItem[];
  inventoryCoverage?: Coverage;
}

export interface ListDevicesQuery {
  id?: string;
  type?: string;
  nodeName?: string;
  nodeUid?: string;
  provider?: string;
}

export interface MigPlacement {
  start: number;
  size: number;
}

export interface MigProfile {
  name: string;
  memoryMb: number;
  sliceCount: number;
  core: number;
  placements: MigPlacement[];
}

export interface DeviceItem {
  uuid: string;
  nodeName: string;
  type: string;
  vgpuUsed: number;
  vgpuTotal: number;
  coreUsed: number;
  coreTotal: number;
  memoryUsed: number;
  memoryTotal: number;
  nodeUid: string;
  health: boolean;
  mode: string;
  coreUsedKnown?: boolean;
  unconfigured: boolean;
  vendor: string;
  migProfiles: MigProfile[];
  nodeAllocationComplete?: boolean;
  hasInitReservation?: boolean;
  telemetryDeviceId?: string;
}

export interface DevicesReply {
  list: DeviceItem[];
}

export interface ContainerDeviceAllocation {
  id: string;
  type: string;
  allocatedCores: number;
  allocatedCoresKnown?: boolean;
  allocatedMem: number;
  allocationShape: string;
  template: string;
  migStart?: number;
  migSize?: number;
  allocationShapeReason: string;
  allocatedCoresReason: string;
}

export interface ContainerStatusDetail {
  containerState: string;
  reason: string;
  message: string;
  ready?: boolean;
  restartCount: number;
  exitCode?: number;
  podPhase: string;
  podReady: string;
  podReadyReason: string;
  podReadyMessage: string;
  restartPending: boolean;
  lastTerminationReason: string;
  lastExitCode?: number;
  lastTerminationMessage: string;
  podReason: string;
  podMessage: string;
}

export interface SchedulingCondition {
  status: string;
  reason: string;
  message: string;
  transitionAt: string;
  messageTruncated: boolean;
}

export interface SchedulingResource {
  name: string;
  value: string;
  unit: string;
  kind: string;
  vendor: string;
}

export interface SchedulingContainerRequest {
  container: string;
  containerKind: string;
  resources: SchedulingResource[];
}

export interface SchedulingConstraint {
  name: string;
  value: string;
}

export interface SchedulingPod {
  name: string;
  namespace: string;
  uid: string;
  createdAt: string;
  schedulerName: string;
  nodeName: string;
  stage: string;
  reasonCodes: string[];
  reasonSource: string;
  condition?: SchedulingCondition | null;
  requests: SchedulingContainerRequest[];
  gates: string[];
  constraints: SchedulingConstraint[];
  preallocated: boolean;
  allocatedContainers: string[];
}

export interface SchedulingEvent {
  uid: string;
  reason: string;
  type: string;
  message: string;
  source: string;
  lastObservedAt: string;
  count: number;
  messageTruncated: boolean;
}

export interface SchedulingPodReply {
  pod: SchedulingPod;
  events: SchedulingEvent[];
  eventStatus: string;
  eventsIncomplete: boolean;
  eventsFetchedAt: string;
}

export interface ContainerItem {
  name: string;
  status: string;
  appName: string;
  nodeName: string;
  allocatedDevices: number;
  allocatedCores: number;
  allocatedMem: number;
  type: string;
  createTime: string;
  startTime: string;
  endTime: string;
  podUid: string;
  nodeUid: string;
  resourcePool: string;
  flavor: string;
  priority: string;
  namespace: string;
  deviceIds: string[];
  images: string[];
  allocatedCoresKnown?: boolean;
  statusDetail?: ContainerStatusDetail | null;
  pending: boolean;
  scheduling?: SchedulingPod | null;
  request?: SchedulingContainerRequest | null;
  containerKind: string;
  allocationShape: string;
  template: string;
  allocatedCoresReason: string;
  vendor: string;
  devices: ContainerDeviceAllocation[];
  allocationShapeReason: string;
  cpuLimitCores?: number;
  memoryLimitBytes?: string | number;
}

export interface NodeContainersReply {
  items: ContainerItem[];
  allocationNodeUid?: string;
}

export interface ListContainersQuery {
  name?: string;
  nodeName?: string;
  status?: string;
  deviceId?: string;
  nodeUid?: string;
  namespace?: string;
  priority?: string;
  keyword?: string;
  page?: number;
  pageSize?: number;
}

export interface ContainersPageReply {
  items: ContainerItem[];
  total: number;
  statusCounts: Record<string, number>;
  nodeCounts: Record<string, number>;
  deviceCounts: Record<string, number>;
}

export interface AscendTemplate {
  name: string;
  memory: string | number;
  aiCore: number;
  aiCpu: number;
  computeShare?: number;
}

export interface AscendModel {
  commonWord: string;
  chipName: string;
  resourceName: string;
  resourceMemoryName: string;
  resourceCoreName: string;
  memoryAllocatable: string | number;
  memoryCapacity: string | number;
  aiCore: number;
  aiCpu: number;
  templates: AscendTemplate[];
  superPod: boolean;
}

export interface DeviceConfigReply {
  state: string;
  reason: string;
  namespace: string;
  name: string;
  key: string;
  resourceVersion: string;
  observedAt: string;
  issues: string[];
  ascendModels: AscendModel[];
}

export interface WebUIBuild {
  version: string;
  commit: string;
}

export interface HamiTarget {
  job: string;
  instance: string;
  pod: string;
  component: string;
  podUid: string;
}

export interface HamiVersion {
  version: string;
  revision: string;
  buildDate: string;
  support: string;
  targets: HamiTarget[];
}

export interface HamiComponent {
  name: string;
  namespace: string;
  pods: number;
  ready: number;
  nodes: number;
  restarts: number;
}

export interface HamiVersions {
  state: string;
  supported: string[];
  versions: HamiVersion[];
  components: HamiComponent[];
}

export interface KubeletVersion {
  version: string;
  nodes: number;
}

export interface PrometheusStatus {
  reachable: boolean;
  metricsRefreshedAt: string;
  version: string;
  refreshQueryFailed: boolean;
}

export interface DevicePlugin {
  id: string;
  vendor: string;
  version: string;
  pods: number;
  ready: number;
}

export interface DeviceExporter {
  id: string;
  vendor: string;
  version: string;
  targets: number;
  up: number;
}

export interface DeviceSources {
  plugins: DevicePlugin[];
  exporters: DeviceExporter[];
  checkedKeys: string[];
  pluginsChecked: boolean;
  exportersChecked: boolean;
}

export interface EntityCountRankingItem {
  name: string;
  nodeUid: string;
  count: number;
}

export interface ContainerAllocationRankingItem {
  podUid: string;
  name: string;
  appName: string;
  namespace: string;
  value: number;
}

export interface WorkloadRankingsReply {
  byNode: EntityCountRankingItem[];
  byDevice: EntityCountRankingItem[];
  byCompute: ContainerAllocationRankingItem[];
  byMemory: ContainerAllocationRankingItem[];
  byVgpu: ContainerAllocationRankingItem[];
}

export interface AboutReply {
  webui: WebUIBuild;
  hami: HamiVersions;
  kubelets: KubeletVersion[];
  prometheus: PrometheusStatus;
  checkedAt: string;
  kubernetesVersion: string;
  deviceSources: DeviceSources;
}

export class ApiClientError extends Error {
  status: number;
  code: number | undefined;
  reason: string;
  metadata: Record<string, string>;
  url: string;

  constructor(options: {
    status: number;
    code?: number;
    reason?: string;
    message?: string;
    metadata?: Record<string, string>;
    url: string;
  });
}

export interface ApiClient {
  getOverviewInventory(options?: RequestOptions): Promise<OverviewInventoryReply>;
  getTelemetryTrends(query: TelemetryTrendsQuery, options?: RequestOptions): Promise<TelemetryTrendsReply>;
  listNodes(query?: ListNodesQuery, options?: RequestOptions): Promise<NodesReply>;
  getNode(uid: string, options?: RequestOptions): Promise<NodeItem>;
  listNodeContainers(uid: string, nodeName: string, options?: RequestOptions): Promise<NodeContainersReply>;
  listDevices(query?: ListDevicesQuery, options?: RequestOptions): Promise<DevicesReply>;
  getDevice(id: string, options?: RequestOptions & { nodeUid?: string }): Promise<DeviceItem>;
  listContainers(query?: ListContainersQuery, options?: RequestOptions): Promise<ContainersPageReply>;
  getContainerRankings(options?: RequestOptions): Promise<WorkloadRankingsReply>;
  getContainer(
    podUid: string,
    container: string,
    options?: RequestOptions & { deviceId?: string },
  ): Promise<ContainerItem>;
  getSchedulingPod(
    namespace: string,
    name: string,
    uid: string,
    options?: RequestOptions,
  ): Promise<SchedulingPodReply>;
  getDeviceConfig(options?: RequestOptions): Promise<DeviceConfigReply>;
  getAbout(options?: RequestOptions): Promise<AboutReply>;
}

export function buildApiUrl(
  path: string,
  query?: Record<string, string | number | boolean | null | undefined>,
  basePath?: string,
): string;

export function createApiClient(options?: {
  fetchImpl?: typeof fetch;
  getBasePathImpl?: () => string;
}): ApiClient;

declare const apiClient: ApiClient;
export default apiClient;
