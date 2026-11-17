export const REPOSITORY_URL = 'https://github.com/Project-HAMi/HAMi-WebUI';

const DOCS_URL = {
  zh: 'https://project-hami.io/zh/docs/userguide/hami-webui-user-guide',
  en: 'https://project-hami.io/docs/userguide/hami-webui-user-guide',
};

const RELEASE_VERSION = /^v?\d+\.\d+\.\d+$/;
const COMMIT = /^[0-9a-f]{7,40}$/;
const HAMI_STATES = ['ok', 'missing', 'unavailable'];
const HAMI_SUPPORT = ['supported', 'unsupported', 'development'];
export const HAMI_COMPONENTS = ['hami-scheduler', 'hami-device-plugin'];
const MAX_NAMES = 3;

const text = (value) => (typeof value === 'string' ? value.trim() : '');
const list = (value) => (Array.isArray(value) ? value : []);
const count = (value) => Number(value) || 0;

export const docsURL = (locale) => DOCS_URL[locale] || DOCS_URL.en;

export const displayVersion = (version) => {
  const value = text(version);
  return /^\d/.test(value) ? `v${value}` : value;
};

export function shortVersion(version) {
  // Strip build metadata but keep release candidates distinct from final releases.
  return version?.replace(/^(v?\d+\.\d+\.\d+(?:-(?:rc|alpha|beta)[.-]?\d+)?)(?:\+.+|-(?:main|master|dev|nightly|build)[.-].+)$/i, '$1');
}

export const describeWebUIBuild = (webui) => {
  if (!webui) return { kind: 'unknown', version: '', commit: '', commitURL: '', label: '' };
  const raw = text(webui.version);
  const fullCommit = COMMIT.test(text(webui.commit)) ? text(webui.commit) : '';
  const commit = fullCommit.slice(0, 7);
  let kind = 'development';
  let version = raw;
  if (RELEASE_VERSION.test(raw)) {
    kind = 'release';
    version = raw.startsWith('v') ? raw : `v${raw}`;
  } else if (!raw || raw === 'dev') {
    kind = 'local';
    version = 'dev';
  }
  return {
    kind,
    version,
    commit,
    commitURL: fullCommit ? `${REPOSITORY_URL}/commit/${fullCommit}` : '',
    label: kind !== 'release' && commit ? `${version} (${commit})` : version,
  };
};

// The API groups by both version and revision. Choose labels across the whole
// cluster so builds remain distinguishable even when they run in different components.
function describeVersions(entries) {
  const revisionsByVersion = new Map();
  for (const entry of entries) {
    const version = text(entry?.version);
    if (!revisionsByVersion.has(version)) revisionsByVersion.set(version, []);
    revisionsByVersion.get(version).push(text(entry?.revision));
  }
  const prefixLengths = new Map();
  for (const [version, revisions] of revisionsByVersion) {
    if (revisions.length < 2) continue;
    const prefixes = new Set(revisions.map(revision => revision.slice(0, 7)));
    prefixLengths.set(version, prefixes.size === revisions.length ? 7 : undefined);
  }
  return entries.map(entry => {
    const version = text(entry?.version);
    const revision = text(entry?.revision);
    const distinguish = revision && prefixLengths.has(version);
    const detailLabel = distinguish ? `${version} (${revision.slice(0, prefixLengths.get(version))})` : version;
    return {
      version, detailLabel, key: JSON.stringify([version, revision]),
      label: !distinguish && version.length > 20 && revision ? revision.slice(0, 7) : detailLabel,
      support: HAMI_SUPPORT.includes(entry?.support) ? entry.support : 'development',
      targets: list(entry?.targets),
    };
  });
}

// One Pod can be scraped by several jobs, so Pods are counted by the UID the server
// matched, and only when it matched one for every target.
const countTargets = (targets) => {
  const pods = new Set(targets.map((target) => text(target?.podUid)));
  return pods.has('') ? { count: targets.length, unit: 'targets' } : { count: pods.size, unit: 'pods' };
};

// Versions reported for one component ('' for unattributed targets), most common first;
// only the others name their Pods.
const componentVersions = (entries, name) => {
  const versions = entries
    .map((entry) => {
      const targets = list(entry?.targets).filter((target) => text(target?.component) === name);
      return { ...entry, ...countTargets(targets), targets };
    })
    .filter((entry) => entry.targets.length)
    .sort((a, b) => b.count - a.count);
  return versions.map(({ targets, ...entry }, index) => {
    const names = index > 0
      ? [...new Set(targets.map((target) => text(target?.pod) || text(target?.instance)).filter(Boolean))]
      : [];
    return {
      ...entry,
      jobs: [...new Set(targets.map((target) => text(target?.job)).filter(Boolean))],
      names: names.slice(0, MAX_NAMES),
      moreNames: Math.max(0, names.length - MAX_NAMES),
    };
  });
};

export const summarizeHami = (hami) => {
  const state = HAMI_STATES.includes(hami?.state) ? hami.state : 'unavailable';
  const versions = describeVersions(list(hami?.versions));
  const components = HAMI_COMPONENTS.map((name) => {
    const found = list(hami?.components).filter((item) => item?.name === name);
    const sum = (field) => found.reduce((total, item) => total + count(item?.[field]), 0);
    const pods = sum('pods');
    const ready = sum('ready');
    return {
      name,
      namespaces: [...new Set(found.map((item) => text(item?.namespace)).filter(Boolean))],
      pods,
      ready,
      restarts: sum('restarts'),
      versions: componentVersions(versions, name),
    };
  });
  const notices = [];
  if (state !== 'ok') notices.push(state);
  if (versions.length > 1) notices.push('mixed');
  if (versions.some((entry) => entry.support === 'unsupported')) notices.push('unsupported');
  if (versions.some((entry) => entry.support === 'development')) notices.push('development');
  return {
    state,
    versions,
    components,
    unattributed: componentVersions(versions, ''),
    notices,
    supported: list(hami?.supported).map(text).filter(Boolean),
  };
};

// A component-wide version requires one build attributed to every Pod.
export function uniformPodVersion(component) {
  const [entry] = component.versions;
  return component.pods > 0 && component.versions.length === 1 &&
    entry.unit === 'pods' && entry.count === component.pods ? entry.version : '';
}

export const kubeletVersions = (about) => list(about?.kubelets)
  .map((entry) => ({ version: text(entry?.version), nodes: count(entry?.nodes) }))
  .filter((entry) => entry.version);

export const refreshAge = (refreshedAt, checkedAt) => {
  const refreshed = Date.parse(refreshedAt);
  const checked = Date.parse(checkedAt);
  if (!Number.isFinite(refreshed) || !Number.isFinite(checked)) return undefined;
  const seconds = Math.max(0, Math.round((checked - refreshed) / 1000));
  if (seconds < 60) return { unit: 'seconds', count: seconds };
  if (seconds < 3600) return { unit: 'minutes', count: Math.floor(seconds / 60) };
  if (seconds < 86400) return { unit: 'hours', count: Math.floor(seconds / 3600) };
  return { unit: 'days', count: Math.floor(seconds / 86400) };
};

// Counts only: CONTRIBUTING asks reporters to keep private cluster data out of issues.
export const issueEnvironmentSummary = (about) => {
  if (!about) return '';
  const build = describeWebUIBuild(about?.webui);
  const hami = summarizeHami(about?.hami);
  const kubelets = kubeletVersions(about)
    .map((entry) => `${entry.version} ×${entry.nodes}`)
    .join(', ');
  const kubernetes = [displayVersion(about?.kubernetesVersion) || 'unknown', kubelets && `kubelet ${kubelets}`]
    .filter(Boolean).join('; ');
  let prometheus = 'unknown';
  if (about?.prometheus?.reachable) prometheus = [displayVersion(about.prometheus.version), 'reachable'].filter(Boolean).join(', ');
  else if (about?.prometheus) prometheus = 'unreachable';
  const units = { pods: ['Pod', 'Pods'], targets: ['scrape target', 'scrape targets'] };
  const versionCounts = (versions) => versions
    .map((item) => `${item.detailLabel} (${item.count} ${units[item.unit][item.count === 1 ? 0 : 1]})`)
    .join(', ');
  const component = (name) => {
    const entry = hami.components.find((item) => item.name === name);
    const pods = entry.pods ? `found ${entry.pods} ${entry.pods === 1 ? 'Pod' : 'Pods'}, ${entry.ready} ready` : 'no Pod records';
    return `${versionCounts(entry.versions) || 'version unknown'}; ${pods}`;
  };
  return [
    `- HAMi-WebUI: ${build.label || 'unknown'}`,
    `- Kubernetes: ${kubernetes}`,
    `- Prometheus: ${prometheus}`,
    `- HAMi scheduler: ${component('hami-scheduler')}`,
    `- HAMi device plugin: ${component('hami-device-plugin')}`,
    ...(hami.unattributed.length ? [`- HAMi, unattributed: ${versionCounts(hami.unattributed)}`] : []),
    ...['plugins', 'exporters'].flatMap(kind => {
      const sources = about.deviceSources;
      if (!sources?.[kind + 'Checked'] || !Array.isArray(sources[kind])) return [];
      return [`- Detected ${kind}: ${sources[kind].length}`];
    }),
  ].join('\n');
};
