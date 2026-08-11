declare module '@mercuryworkshop/proxy-transports' {
  export type RawHeaders = [string, string][];

  export type TransferrableResponse = {
    body: ReadableStream<Uint8Array> | ArrayBuffer;
    headers: RawHeaders;
    status: number;
    statusText: string;
  };

  export interface ProxyTransport {
    ready: boolean;
    init(): Promise<void>;
    request(
      remote: URL,
      method: string,
      body: BodyInit | null,
      headers: RawHeaders,
      signal: AbortSignal | undefined,
    ): Promise<TransferrableResponse>;
    connect(
      url: URL,
      protocols: string[],
      requestHeaders: RawHeaders,
      onopen: (protocol: string, extensions: string) => void,
      onmessage: (data: Blob | ArrayBuffer | string) => void,
      onclose: (code: number, reason: string) => void,
      onerror: (error: string) => void,
    ): [(data: Blob | ArrayBuffer | string) => void, (code: number, reason: string) => void];
  }
}

declare module '@mercuryworkshop/epoxy-tls' {
  import type { RawHeaders } from '@mercuryworkshop/proxy-transports';

  export default function initialize(): Promise<void>;

  export class EpoxyClientOptions {
    user_agent: string;
    wisp_v2: boolean;
    udp_extension_required: boolean;
    pem_files: string[];
    disable_certificate_validation: boolean;
  }

  export class EpoxyHandlers {
    constructor(
      open: () => void,
      close: () => void,
      error: (error: string) => void,
      message: (data: Uint8Array | string) => void,
    );
  }

  export class EpoxyClient {
    constructor(factory: () => unknown, options: EpoxyClientOptions);
    fetch(
      url: string,
      options: {
        method: string;
        body: BodyInit | null;
        headers: Record<string, string>;
        redirect: 'manual';
      },
    ): Promise<
      Response & { rawHeaders?: RawHeaders | Headers | Record<string, string | string[]> }
    >;
    connect_websocket(
      handlers: EpoxyHandlers,
      url: string,
      protocols: string[],
      headers: Record<string, string>,
    ): Promise<{
      send(data: ArrayBuffer | string): void;
      close(code: number, reason: string): void;
    }>;
  }
}

declare module '@mercuryworkshop/scramjet-controller' {
  import type { ProxyTransport } from '@mercuryworkshop/proxy-transports';

  export interface Frame {
    go(url: string): void;
  }

  export class Controller {
    constructor(options: {
      serviceworker: ServiceWorker;
      transport: ProxyTransport;
      config: { scramjetPath: string; injectPath: string; wasmPath: string };
    });
    wait(): Promise<void>;
    setTransport(transport: ProxyTransport): void;
    createFrame(element: HTMLIFrameElement): Frame;
  }
}
