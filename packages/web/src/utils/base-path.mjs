const rootBasePath = '/';

/**
 * Return the runtime URL prefix declared by the document's <base> element.
 *
 * The Web entry rewrites that element when HAMi-WebUI is mounted below the
 * origin root. Reading document.baseURI keeps the router and same-origin API
 * client on the same prefix without baking deployment-specific values into the
 * frontend bundle.
 */
export function getBasePath(baseURI = globalThis.document?.baseURI) {
  if (!baseURI) return rootBasePath;

  try {
    const pathname = new URL(baseURI).pathname;
    if (!pathname.startsWith('/')) return rootBasePath;
    return pathname.endsWith('/') ? pathname : `${pathname}/`;
  } catch {
    return rootBasePath;
  }
}
