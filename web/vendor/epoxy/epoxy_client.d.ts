/* tslint:disable */
/* eslint-disable */
/**
 * The `ReadableStreamType` enum.
 *
 * *This API requires the following crate features to be activated: `ReadableStreamType`*
 */
type ReadableStreamType = "bytes";

type EpoxyIoStream = {
	read: ReadableStream<Uint8Array>,
	write: WritableStream<Uint8Array>,
};
type EpoxyWispTransportResult = { read: ReadableStream<ArrayBuffer>, write: WritableStream<Uint8Array> };
type EpoxyWispTransport = string | (() => Promise<EpoxyWispTransportResult> | EpoxyWispTransportResult);
type EpoxyWebSocketInput = string | ArrayBuffer;
type EpoxyWebSocketHeadersInput = Headers | { [key: string]: string };
type EpoxyUrlInput = string | URL;


export class EpoxyClient {
/**
** Return copy of self without private attributes.
*/
  toJSON(): Object;
/**
* Return stringified version of self.
*/
  toString(): string;
  free(): void;
  connect_tcp(url: EpoxyUrlInput): Promise<EpoxyIoStream>;
  connect_tls(url: EpoxyUrlInput): Promise<EpoxyIoStream>;
  connect_udp(url: EpoxyUrlInput): Promise<EpoxyIoStream>;
  connect_websocket(handlers: EpoxyHandlers, url: EpoxyUrlInput, protocols: string[], headers: EpoxyWebSocketHeadersInput): Promise<EpoxyWebSocket>;
  replace_stream_provider(): Promise<void>;
  fetch(url: EpoxyUrlInput, options: object): Promise<Response>;
  constructor(transport: EpoxyWispTransport, options: EpoxyClientOptions);
  redirect_limit: number;
  user_agent: string;
  buffer_size: number;
}
export class EpoxyClientOptions {
  free(): void;
  constructor();
  wisp_v2: boolean;
  udp_extension_required: boolean;
  title_case_headers: boolean;
  ws_title_case_headers: boolean;
  websocket_protocols: string[];
  redirect_limit: number;
  header_limit: number;
  user_agent: string;
  pem_files: string[];
  disable_certificate_validation: boolean;
  buffer_size: number;
}
export class EpoxyHandlers {
  free(): void;
  constructor(onopen: Function, onclose: Function, onerror: Function, onmessage: Function);
  onopen: Function;
  onclose: Function;
  onerror: Function;
  onmessage: Function;
}
export class EpoxyWebSocket {
  private constructor();
  free(): void;
  send(payload: EpoxyWebSocketInput): Promise<void>;
  close(code: number, reason: string): Promise<void>;
}
export class IntoUnderlyingByteSource {
  private constructor();
  free(): void;
  pull(controller: ReadableByteStreamController): Promise<any>;
  start(controller: ReadableByteStreamController): void;
  cancel(): void;
  readonly autoAllocateChunkSize: number;
  readonly type: ReadableStreamType;
}
export class IntoUnderlyingSink {
  private constructor();
  free(): void;
  abort(reason: any): Promise<any>;
  close(): Promise<any>;
  write(chunk: any): Promise<any>;
}
export class IntoUnderlyingSource {
  private constructor();
  free(): void;
  pull(controller: ReadableStreamDefaultController): Promise<any>;
  cancel(): void;
}

export type InitInput = RequestInfo | URL | Response | BufferSource | WebAssembly.Module;

export interface InitOutput {
  readonly memory: WebAssembly.Memory;
  readonly __wbg_epoxyclient_free: (a: number, b: number) => void;
  readonly __wbg_epoxyclientoptions_free: (a: number, b: number) => void;
  readonly __wbg_epoxyhandlers_free: (a: number, b: number) => void;
  readonly __wbg_epoxywebsocket_free: (a: number, b: number) => void;
  readonly __wbg_get_epoxyclient_buffer_size: (a: number) => number;
  readonly __wbg_get_epoxyclient_redirect_limit: (a: number) => number;
  readonly __wbg_get_epoxyclient_user_agent: (a: number) => [number, number];
  readonly __wbg_get_epoxyclientoptions_buffer_size: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_disable_certificate_validation: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_header_limit: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_pem_files: (a: number) => [number, number];
  readonly __wbg_get_epoxyclientoptions_redirect_limit: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_title_case_headers: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_udp_extension_required: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_user_agent: (a: number) => [number, number];
  readonly __wbg_get_epoxyclientoptions_websocket_protocols: (a: number) => [number, number];
  readonly __wbg_get_epoxyclientoptions_wisp_v2: (a: number) => number;
  readonly __wbg_get_epoxyclientoptions_ws_title_case_headers: (a: number) => number;
  readonly __wbg_get_epoxyhandlers_onclose: (a: number) => number;
  readonly __wbg_get_epoxyhandlers_onerror: (a: number) => number;
  readonly __wbg_get_epoxyhandlers_onmessage: (a: number) => number;
  readonly __wbg_get_epoxyhandlers_onopen: (a: number) => number;
  readonly __wbg_set_epoxyclient_buffer_size: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclient_redirect_limit: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclient_user_agent: (a: number, b: number, c: number) => void;
  readonly __wbg_set_epoxyclientoptions_buffer_size: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_disable_certificate_validation: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_header_limit: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_pem_files: (a: number, b: number, c: number) => void;
  readonly __wbg_set_epoxyclientoptions_redirect_limit: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_title_case_headers: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_udp_extension_required: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_user_agent: (a: number, b: number, c: number) => void;
  readonly __wbg_set_epoxyclientoptions_websocket_protocols: (a: number, b: number, c: number) => void;
  readonly __wbg_set_epoxyclientoptions_wisp_v2: (a: number, b: number) => void;
  readonly __wbg_set_epoxyclientoptions_ws_title_case_headers: (a: number, b: number) => void;
  readonly __wbg_set_epoxyhandlers_onclose: (a: number, b: number) => void;
  readonly __wbg_set_epoxyhandlers_onerror: (a: number, b: number) => void;
  readonly __wbg_set_epoxyhandlers_onmessage: (a: number, b: number) => void;
  readonly __wbg_set_epoxyhandlers_onopen: (a: number, b: number) => void;
  readonly epoxyclient_connect_tcp: (a: number, b: number) => number;
  readonly epoxyclient_connect_tls: (a: number, b: number) => number;
  readonly epoxyclient_connect_udp: (a: number, b: number) => number;
  readonly epoxyclient_connect_websocket: (a: number, b: number, c: number, d: number, e: number, f: number) => number;
  readonly epoxyclient_fetch: (a: number, b: number, c: number) => number;
  readonly epoxyclient_new: (a: number, b: number) => [number, number, number];
  readonly epoxyclient_replace_stream_provider: (a: number) => number;
  readonly epoxyclientoptions_new_default: () => number;
  readonly epoxyhandlers_new: (a: number, b: number, c: number, d: number) => number;
  readonly epoxywebsocket_close: (a: number, b: number, c: number, d: number) => number;
  readonly epoxywebsocket_send: (a: number, b: number) => number;
  readonly ring_core_0_17_14__bn_mul_mont: (a: number, b: number, c: number, d: number, e: number, f: number) => void;
  readonly __wbg_intounderlyingbytesource_free: (a: number, b: number) => void;
  readonly __wbg_intounderlyingsink_free: (a: number, b: number) => void;
  readonly __wbg_intounderlyingsource_free: (a: number, b: number) => void;
  readonly intounderlyingbytesource_autoAllocateChunkSize: (a: number) => number;
  readonly intounderlyingbytesource_cancel: (a: number) => void;
  readonly intounderlyingbytesource_pull: (a: number, b: number) => number;
  readonly intounderlyingbytesource_start: (a: number, b: number) => void;
  readonly intounderlyingbytesource_type: (a: number) => number;
  readonly intounderlyingsink_abort: (a: number, b: number) => number;
  readonly intounderlyingsink_close: (a: number) => number;
  readonly intounderlyingsink_write: (a: number, b: number) => number;
  readonly intounderlyingsource_cancel: (a: number) => void;
  readonly intounderlyingsource_pull: (a: number, b: number) => number;
  readonly __wbindgen_exn_store: (a: number) => void;
  readonly __wbindgen_malloc: (a: number, b: number) => number;
  readonly __wbindgen_realloc: (a: number, b: number, c: number, d: number) => number;
  readonly __wbindgen_export_3: WebAssembly.Table;
  readonly __wbindgen_free: (a: number, b: number, c: number) => void;
  readonly _dyn_core_9a9629a4e52e66ad___ops__function__Fn_______Output______as_wasm_bindgen_7ef8d7b19f376ee9___closure__WasmClosure___describe__invoke___wasm_bindgen_7ef8d7b19f376ee9___JsValue_____: (a: number, b: number, c: number) => void;
  readonly _dyn_core_9a9629a4e52e66ad___ops__function__Fn_____Output______as_wasm_bindgen_7ef8d7b19f376ee9___closure__WasmClosure___describe__invoke______: (a: number, b: number) => void;
  readonly _dyn_core_9a9629a4e52e66ad___ops__function__FnMut_______Output______as_wasm_bindgen_7ef8d7b19f376ee9___closure__WasmClosure___describe__invoke___wasm_bindgen_7ef8d7b19f376ee9___JsValue_____: (a: number, b: number, c: number) => void;
  readonly _dyn_core_9a9629a4e52e66ad___ops__function__FnMut_____Output______as_wasm_bindgen_7ef8d7b19f376ee9___closure__WasmClosure___describe__invoke______: (a: number, b: number) => void;
  readonly wasm_bindgen_7ef8d7b19f376ee9___convert__closures__invoke2_mut___wasm_bindgen_7ef8d7b19f376ee9___JsValue__wasm_bindgen_7ef8d7b19f376ee9___JsValue_____: (a: number, b: number, c: number, d: number) => void;
}

export type SyncInitInput = BufferSource | WebAssembly.Module;
/**
* Instantiates the given `module`, which can either be bytes or
* a precompiled `WebAssembly.Module`.
*
* @param {{ module: SyncInitInput }} module - Passing `SyncInitInput` directly is deprecated.
*
* @returns {InitOutput}
*/
export function initSync(module: { module: SyncInitInput } | SyncInitInput): InitOutput;

/**
* If `module_or_path` is {RequestInfo} or {URL}, makes a request and
* for everything else, calls `WebAssembly.instantiate` directly.
*
* @param {{ module_or_path: InitInput | Promise<InitInput> }} module_or_path - Passing `InitInput` directly is deprecated.
*
* @returns {Promise<InitOutput>}
*/
export default function __wbg_init (module_or_path?: { module_or_path: InitInput | Promise<InitInput> } | InitInput | Promise<InitInput>): Promise<InitOutput>;
