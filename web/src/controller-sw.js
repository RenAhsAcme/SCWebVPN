importScripts('/compat/controller/controller.lib.js');

const proxyPathPrefix = '/~/sj/';
const routeReviveMaxAttempts = 20;
const routeReviveDelayMs = 100;
const localBinaryDataMaxLength = 1024 * 1024;

const delay = (milliseconds) =>
  new Promise((resolve) => {
    setTimeout(resolve, milliseconds);
  });

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

function localBinaryDataURL(requestUrl) {
  const match = requestUrl.pathname.match(/^\/~\/sj\/[^/]+\/[^/]+\/(data:.+)$/);
  if (!match) return null;

  let dataURL;
  try {
    dataURL = decodeURIComponent(match[1]);
  } catch {
    return null;
  }

  if (dataURL.length > localBinaryDataMaxLength) return null;
  return /^data:application\/(?:octet-stream|wasm);base64,[A-Za-z0-9+/=]+$/.test(dataURL)
    ? dataURL
    : null;
}

function localBinaryDataResponse(dataURL) {
  try {
    const encoded = dataURL.slice(dataURL.indexOf(',') + 1);
    const decoded = atob(encoded);
    const bytes = new Uint8Array(decoded.length);
    for (let index = 0; index < decoded.length; index += 1) {
      bytes[index] = decoded.charCodeAt(index);
    }

    return new Response(bytes, {
      status: 200,
      headers: {
        'Cache-Control': 'no-store',
        'Content-Type': dataURL.startsWith('data:application/wasm;')
          ? 'application/wasm'
          : 'application/octet-stream',
      },
    });
  } catch {
    return new Response('Invalid embedded binary data.', {
      status: 400,
      headers: {
        'Cache-Control': 'no-store',
        'Content-Type': 'text/plain; charset=utf-8',
      },
    });
  }
}

async function routeProxyRequest(event) {
  for (let attempt = 0; attempt < routeReviveMaxAttempts; attempt += 1) {
    if (self.$scramjetController.shouldRoute(event)) {
      return self.$scramjetController.route(event);
    }

    await delay(routeReviveDelayMs);
  }

  return new Response('WebVPN proxy route is temporarily unavailable.', {
    status: 503,
    headers: {
      'Cache-Control': 'no-store',
      'Content-Type': 'text/plain; charset=utf-8',
    },
  });
}

self.addEventListener('fetch', (event) => {
  const requestUrl = new URL(event.request.url);
  if (
    requestUrl.origin === self.location.origin &&
    requestUrl.pathname.startsWith(proxyPathPrefix)
  ) {
    const localDataURL = localBinaryDataURL(requestUrl);
    if (localDataURL) {
      event.respondWith(localBinaryDataResponse(localDataURL));
      return;
    }

    event.respondWith(routeProxyRequest(event));
    return;
  }

  if (self.$scramjetController.shouldRoute(event)) {
    event.respondWith(self.$scramjetController.route(event));
  }
});
