import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { optimize } from 'svgo';

const virtualModuleId = 'virtual:svg-icons-register';
const resolvedVirtualModuleId = `\0${virtualModuleId}`;
const spriteDomId = '__svg__icons__dom__';
const svgNamespace = 'http://www.w3.org/2000/svg';

const toPosixPath = (value) => value.split(path.sep).join('/');

const isInsideDirectory = (file, directory) => {
  const relative = path.relative(directory, file);
  return (
    relative !== '' &&
    relative !== '..' &&
    !relative.startsWith(`..${path.sep}`) &&
    !path.isAbsolute(relative)
  );
};

const listSvgFiles = async (directory) => {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const absolutePath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listSvgFiles(absolutePath)));
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith('.svg')) {
      files.push(absolutePath);
    }
  }

  return files.sort((left, right) =>
    toPosixPath(left).localeCompare(toPosixPath(right), 'en'),
  );
};

const preserveCurrentColorContract = () => {
  let replacedStroke = false;

  return {
    name: 'hami-preserve-current-color-contract',
    fn: () => ({
      element: {
        enter(node) {
          const stroke = node.attributes.stroke;
          if (
            !replacedStroke &&
            stroke !== undefined &&
            /^[A-Za-z#0-9]*$/.test(stroke)
          ) {
            node.attributes.stroke = 'currentColor';
            replacedStroke = true;
          }
        },
      },
    }),
  };
};

const convertSvgToSymbol = (symbolId) => ({
  name: 'hami-convert-svg-to-symbol',
  fn: () => ({
    element: {
      enter(node, parentNode) {
        if (node.name !== 'svg' || parentNode.type !== 'root') return;

        const preservedAttributes = Object.fromEntries(
          Object.entries(node.attributes).filter(([name]) =>
            /^(?:viewBox|preserveAspectRatio|class|overflow|role|aria-.+|(?:stroke|fill)(?:-.+)?)$/.test(
              name,
            ),
          ),
        );

        node.name = 'symbol';
        node.attributes = { ...preservedAttributes, id: symbolId };
      },
    },
  }),
});

export const createSvgSymbolId = (relativePath) =>
  `icon-${toPosixPath(relativePath).replace(/\.svg$/i, '')}`;

export const compileSvgSymbol = (source, file, symbolId) => {
  const internalIdPrefix = symbolId.replace(/[^A-Za-z0-9_-]/g, '_');
  const result = optimize(source, {
    path: file,
    plugins: [
      'preset-default',
      preserveCurrentColorContract(),
      {
        name: 'prefixIds',
        params: {
          prefix: internalIdPrefix,
          delim: '_',
          prefixClassNames: false,
        },
      },
      'removeDimensions',
      'removeScripts',
      convertSvgToSymbol(symbolId),
    ],
  });

  if (!result.data.startsWith('<symbol')) {
    throw new Error(`Expected one SVG root in ${file}`);
  }

  return result.data;
};

export const buildSvgSprite = async (iconDir) => {
  const directory = path.resolve(iconDir);
  const files = await listSvgFiles(directory);
  const symbols = [];
  const symbolSources = new Map();
  const internalIdSources = new Map();

  for (const file of files) {
    const relativePath = toPosixPath(path.relative(directory, file));
    const symbolId = createSvgSymbolId(relativePath);
    const existingSource = symbolSources.get(symbolId);

    if (existingSource) {
      throw new Error(
        `Duplicate SVG symbol ID ${symbolId}: ${existingSource} and ${file}`,
      );
    }

    const source = await readFile(file, 'utf8');
    const symbol = compileSvgSymbol(source, file, symbolId);

    for (const match of symbol.matchAll(/\sid="([^"]+)"/g)) {
      const internalId = match[1];
      if (internalId === symbolId) continue;

      const existingInternalSource = internalIdSources.get(internalId);
      if (existingInternalSource) {
        throw new Error(
          `Duplicate internal SVG ID ${internalId}: ${existingInternalSource} and ${file}`,
        );
      }
      internalIdSources.set(internalId, file);
    }

    symbolSources.set(symbolId, file);
    symbols.push(symbol);
  }

  return {
    files,
    symbolIds: [...symbolSources.keys()],
    sprite: symbols.join(''),
  };
};

const createRegistrationModule = (sprite) => `
if (typeof window !== 'undefined') {
  function loadSvg() {
    var body = document.body;
    var svgDom = document.getElementById('${spriteDomId}');
    if (!svgDom) {
      svgDom = document.createElementNS('${svgNamespace}', 'svg');
      svgDom.style.position = 'absolute';
      svgDom.style.width = '0';
      svgDom.style.height = '0';
      svgDom.id = '${spriteDomId}';
      svgDom.setAttribute('xmlns', '${svgNamespace}');
    }
    svgDom.innerHTML = ${JSON.stringify(sprite)};
    body.insertBefore(svgDom, body.lastChild);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', loadSvg);
  } else {
    loadSvg();
  }
}
export default {};
`;

export const createSvgIconsPlugin = ({ iconDir }) => {
  const directory = path.resolve(iconDir);

  return {
    name: 'hami-svg-icons',
    resolveId(id) {
      return id === virtualModuleId ? resolvedVirtualModuleId : null;
    },
    async load(id) {
      if (id !== resolvedVirtualModuleId) return null;

      const { files, sprite } = await buildSvgSprite(directory);
      for (const file of files) this.addWatchFile(file);
      return createRegistrationModule(sprite);
    },
    configureServer(server) {
      server.watcher.add(directory);
      const reloadSprite = (file) => {
        if (!file.toLowerCase().endsWith('.svg')) return;
        if (!isInsideDirectory(path.resolve(file), directory)) return;

        const module = server.moduleGraph.getModuleById(
          resolvedVirtualModuleId,
        );
        if (module) server.moduleGraph.invalidateModule(module);
        server.ws.send({ type: 'full-reload' });
      };

      server.watcher.on('add', reloadSprite);
      server.watcher.on('change', reloadSprite);
      server.watcher.on('unlink', reloadSprite);
    },
  };
};
