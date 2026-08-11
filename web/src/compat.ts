import initEpoxy, {
  EpoxyClient,
  EpoxyClientOptions,
  EpoxyHandlers,
} from '@mercuryworkshop/epoxy-tls';
import type {
  ProxyTransport,
  RawHeaders,
  TransferrableResponse,
} from '@mercuryworkshop/proxy-transports';

import { RTCWispTransport } from './rtc-stream';
import { agentTransportURL, sanitizeUpstreamHeaders } from './transport-policy';

export class RTCEpoxyTransport implements ProxyTransport {
  ready = false;
  private client: EpoxyClient | null = null;
  private fetchQueue: Promise<void> = Promise.resolve();

  constructor(private readonly byteTransport: RTCWispTransport) {}

  async init() {
    if (this.ready) return;
    await initEpoxy();
    const options = new EpoxyClientOptions();
    options.user_agent = navigator.userAgent;
    options.wisp_v2 = true;
    options.udp_extension_required = false;
    options.pem_files = [];
    options.disable_certificate_validation = false;
    this.client = new EpoxyClient(this.byteTransport.factory, options);
    this.ready = true;
  }

  async request(
    remote: URL,
    method: string,
    body: BodyInit | null,
    headers: RawHeaders,
    _signal: AbortSignal | undefined,
  ): Promise<TransferrableResponse> {
    await this.init();
    if (body instanceof Blob) body = await body.arrayBuffer();
    const pending = this.fetchQueue.then(() =>
      this.client!.fetch(agentTransportURL(remote).href, {
        method,
        body,
        headers: Object.fromEntries(headers),
        redirect: 'manual',
      }),
    );
    this.fetchQueue = pending.then(
      () => undefined,
      () => undefined,
    );
    const response = await pending;
    const responseHeaderList = responseHeaders(response, remote);
    const contentType =
      responseHeaderList.find(([name]) => name.toLowerCase() === 'content-type')?.[1] ?? '';
    const responseBody = /^(?:text\/html|application\/xhtml\+xml)(?:;|$)/i.test(contentType)
      ? await response.arrayBuffer()
      : response.body!;
    return {
      body: responseBody,
      headers: responseHeaderList,
      status: response.status,
      statusText: response.statusText,
    };
  }

  connect(
    url: URL,
    protocols: string[],
    requestHeaders: RawHeaders,
    onopen: (protocol: string, extensions: string) => void,
    onmessage: (data: Blob | ArrayBuffer | string) => void,
    onclose: (code: number, reason: string) => void,
    onerror: (error: string) => void,
  ): [(data: Blob | ArrayBuffer | string) => void, (code: number, reason: string) => void] {
    const handlers = new EpoxyHandlers(
      () => onopen('', ''),
      () => onclose(1000, 'Closed by remote'),
      onerror,
      (data: Uint8Array | string) =>
        onmessage(data instanceof Uint8Array ? (data.slice().buffer as ArrayBuffer) : data),
    );
    const socket = this.client!.connect_websocket(
      handlers,
      agentTransportURL(url).href,
      protocols,
      Object.fromEntries(requestHeaders),
    );
    return [
      async (data) => (await socket).send(data instanceof Blob ? await data.arrayBuffer() : data),
      async (code, reason) => (await socket).close(code, reason || ''),
    ];
  }
}

function responseHeaders(response: Response & { rawHeaders?: unknown }, remote: URL): RawHeaders {
  const values: RawHeaders = [];
  const raw = response.rawHeaders;
  if (Array.isArray(raw)) {
    for (const item of raw) {
      if (Array.isArray(item) && item.length >= 2) values.push([String(item[0]), String(item[1])]);
    }
  } else if (raw instanceof Headers) {
    values.push(...raw.entries());
  } else if (raw && typeof raw === 'object') {
    for (const [name, value] of Object.entries(raw)) {
      for (const item of Array.isArray(value) ? value : [value]) {
        if (item != null) values.push([name, String(item)]);
      }
    }
  }
  const normalized = values.length ? values : [...response.headers.entries()];
  return sanitizeUpstreamHeaders(normalized, remote);
}
