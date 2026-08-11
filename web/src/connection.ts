import { RTCWispTransport } from './rtc-stream';
import { parseBindingCode, type BindingCode } from './binding';

const maxRestarts = 2;
const authOK = 'webvpn-auth-ok-v1';
const authReady = 'webvpn-auth-ready-v1';
const allowedCandidateTypes = new Set(['host', 'srflx', 'prflx']);

type Description = { type: RTCSdpType; sdp: string };
type ConnectionStatus = {
  state: 'pending' | 'answered' | 'failed';
  answer?: Description;
  failure_code?: string;
};
type CreatedConnection = {
  id: string;
  browser_session_id: string;
  capability: string;
};
type CandidateStats = RTCStats & { candidateType: string; protocol?: string };
export type DirectStats = {
  localType: string;
  remoteType: string;
  protocol: string;
  rttMS: number | null;
  hostCandidate: boolean;
  srflxCandidate: boolean;
  dataChannels: 'open';
};
export type DirectConnectionHooks = {
  status(message: string): void;
  stats(value: DirectStats | null): void;
  failed(error: Error): void;
};
export type DirectSession = {
  wisp: RTCWispTransport;
  control: ControlClient;
};
export type DiagnosticResult = {
  code: 'ok' | 'blocked' | 'unavailable' | 'busy' | 'invalid_request';
  addresses?: string[];
  rtt_ms?: number;
};
export type TemporaryTarget = {
  host: string;
  port: number;
  kind: 'http' | 'https';
};

export class DirectConnection {
  private peer: RTCPeerConnection | null = null;
  private signalID = '';
  private browserSessionID = '';
  private capability = '';
  private serviceID = '';
  private restartCount = 0;
  private restarting = false;
  private closed = false;
  private candidateTypes = new Set<string>();

  constructor(
    private readonly stunURLs: readonly string[],
    private readonly hooks: DirectConnectionHooks,
  ) {
    validateSTUN(stunURLs);
  }

  async connect(serviceID: string, temporaryTarget?: TemporaryTarget): Promise<DirectSession> {
    this.close();
    this.closed = false;
    this.serviceID = serviceID;
    this.candidateTypes.clear();
    this.hooks.status('正在收集候选');
    const peer = new RTCPeerConnection({
      iceServers: [{ urls: [...this.stunURLs] }],
      iceTransportPolicy: 'all',
      bundlePolicy: 'max-bundle',
    });
    this.peer = peer;
    const control = reliableChannel(peer, 'webvpn-control-v1');
    const interactive = reliableChannel(peer, 'wisp-interactive-v2');
    const bulk = reliableChannel(peer, 'wisp-bulk-v2');
    this.observe(peer);

    const offer = await this.localOffer(false);
    const created = await api<CreatedConnection>('/api/v1/browser/connections', {
      method: 'POST',
      body: JSON.stringify({ service_id: serviceID, offer }),
    });
    this.signalID = created.id;
    this.browserSessionID = created.browser_session_id;
    this.capability = created.capability;
    const answer = await this.waitForAnswer(created.id);
    rejectRelay(answer.sdp);
    await peer.setRemoteDescription(answer);
    this.hooks.status('正在建立 P2P');

    await Promise.all([open(control), open(interactive), open(bulk)]);
    const [interactiveWisp, , controlClient] = await Promise.all([
      authenticateWisp(interactive, this.capability),
      authenticateWisp(bulk, this.capability),
      authenticateControl(control, this.capability),
    ]);
    if (temporaryTarget) await controlClient.configureTemporary(temporaryTarget);
    const stats = await selectedPair(peer, this.candidateTypes);
    this.hooks.stats(stats);
    this.hooks.status(
      `P2P UDP · ${stats.localType === 'host' && stats.remoteType === 'host' ? '局域网直连' : '公网映射直连'}`,
    );
    return { wisp: interactiveWisp, control: controlClient };
  }

  async restart(): Promise<void> {
    const peer = this.peer;
    if (!peer || this.closed || this.restarting) return;
    if (this.restartCount >= maxRestarts) {
      this.fail(new Error('ICE 重启次数已耗尽'));
      return;
    }
    this.restarting = true;
    this.restartCount++;
    this.hooks.status(`ICE 重启中（${this.restartCount}/${maxRestarts}）`);
    try {
      this.candidateTypes.clear();
      peer.restartIce();
      const offer = await this.localOffer(true);
      const created = await api<CreatedConnection>('/api/v1/browser/connections', {
        method: 'POST',
        body: JSON.stringify({
          service_id: this.serviceID,
          browser_session_id: this.browserSessionID,
          capability: this.capability,
          offer,
          restart_of: this.signalID,
        }),
      });
      const answer = await this.waitForAnswer(created.id);
      rejectRelay(answer.sdp);
      await peer.setRemoteDescription(answer);
      await connected(peer, 20_000);
      this.signalID = created.id;
      const stats = await selectedPair(peer, this.candidateTypes);
      this.hooks.stats(stats);
      this.hooks.status(
        `P2P UDP · ${stats.localType === 'host' && stats.remoteType === 'host' ? '局域网直连' : '公网映射直连'}`,
      );
    } catch (error) {
      if (this.restartCount < maxRestarts && !this.closed) {
        await delay(900);
        this.restarting = false;
        await this.restart();
        return;
      }
      this.fail(asError(error));
    } finally {
      this.restarting = false;
    }
  }

  close() {
    this.closed = true;
    this.peer?.close();
    this.peer = null;
    this.signalID = '';
    this.browserSessionID = '';
    this.capability = '';
    this.serviceID = '';
    this.restartCount = 0;
    this.hooks.stats(null);
  }

  private async localOffer(restart: boolean): Promise<Description> {
    const peer = this.peer;
    if (!peer) throw new Error('P2P connection is closed');
    const offer = await peer.createOffer(restart ? { iceRestart: true } : undefined);
    await peer.setLocalDescription(offer);
    await Promise.race([gathered(peer), delay(12_000)]);
    const local = peer.localDescription;
    if (!local?.sdp) throw new Error('浏览器未生成有效的连接描述');
    rejectRelay(local.sdp);
    return { type: local.type, sdp: local.sdp };
  }

  private async waitForAnswer(id: string): Promise<Description> {
    const deadline = Date.now() + 45_000;
    while (!this.closed && Date.now() < deadline) {
      const status = await api<ConnectionStatus>(
        `/api/v1/browser/connections/${encodeURIComponent(id)}`,
      );
      if (status.state === 'answered' && status.answer) return status.answer;
      if (status.state === 'failed') throw new Error(failureMessage(status.failure_code));
      await delay(500);
    }
    throw new Error('等待 OpenWrt Agent 应答超时');
  }

  private observe(peer: RTCPeerConnection) {
    peer.addEventListener('icecandidate', (event) => {
      if (event.candidate?.type) this.candidateTypes.add(event.candidate.type);
      if (event.candidate?.type === 'relay') this.fail(new Error('检测到被禁止的 relay candidate'));
    });
    peer.addEventListener('connectionstatechange', () => {
      if (this.closed || peer !== this.peer) return;
      if (peer.connectionState === 'disconnected' || peer.connectionState === 'failed') {
        window.setTimeout(() => {
          if (peer === this.peer && peer.connectionState !== 'connected') void this.restart();
        }, 1_500);
      }
    });
  }

  private fail(error: Error) {
    this.close();
    this.hooks.failed(error);
  }
}

async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    cache: 'no-store',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...init.headers },
  });
  const body = response.status === 204 ? null : await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body?.error || `控制面请求失败（${response.status}）`);
  return body as T;
}

export async function browserConfig(): Promise<{ stun_urls: string[] }> {
  const value = await api<{ stun_urls: string[] }>('/api/v1/browser/config');
  validateSTUN(value.stun_urls);
  return value;
}

export async function services<T>(): Promise<T[]> {
  return (await api<{ services: T[] }>('/api/v1/browser/services')).services;
}

export async function createBindingCode(): Promise<BindingCode> {
  return parseBindingCode(
    await api<unknown>('/api/v1/browser/binding-codes', {
      method: 'POST',
      body: '{}',
    }),
  );
}

export async function createTemporaryService<T>(input: {
  agent_id: string;
  display_name: string;
  kind: 'http' | 'https';
}): Promise<T> {
  return api<T>('/api/v1/browser/temporary-services', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function currentTemporaryService<T>(): Promise<T> {
  return api<T>('/api/v1/browser/temporary-services/current');
}

export async function agentOnline(agentID: string): Promise<boolean> {
  return (
    await api<{ online: boolean }>(`/api/v1/browser/agents/${encodeURIComponent(agentID)}/status`)
  ).online;
}

function reliableChannel(peer: RTCPeerConnection, label: string): RTCDataChannel {
  return peer.createDataChannel(label, { ordered: true });
}

async function authenticateWisp(
  channel: RTCDataChannel,
  capability: string,
): Promise<RTCWispTransport> {
  const authenticated = textMessage(channel, authOK, 5_000);
  channel.send(`webvpn-auth-v1:${capability}`);
  await authenticated;
  const transport = new RTCWispTransport(channel);
  channel.send(authReady);
  return transport;
}

async function authenticateControl(
  channel: RTCDataChannel,
  capability: string,
): Promise<ControlClient> {
  const authenticated = textMessage(channel, authOK, 5_000);
  channel.send(`webvpn-auth-v1:${capability}`);
  await authenticated;
  const pong = textMessage(channel, '{"type":"pong"}', 5_000);
  channel.send('{"type":"ping"}');
  await pong;
  return new ControlClient(channel);
}

export class ControlClient {
  private pending = new Map<
    string,
    {
      resolve: (value: DiagnosticResult) => void;
      reject: (error: Error) => void;
      timer: number;
      expectedType: 'diagnostic_result' | 'temporary_result';
    }
  >();

  constructor(private readonly channel: RTCDataChannel) {
    channel.addEventListener('message', (event) => this.receive(event.data));
    channel.addEventListener('close', () => this.failAll(new Error('诊断通道已关闭')));
    channel.addEventListener('error', () => this.failAll(new Error('诊断通道发生错误')));
  }

  dns(name: string): Promise<DiagnosticResult> {
    return this.request({ type: 'dns', name });
  }

  ping(): Promise<DiagnosticResult> {
    return this.request({ type: 'diagnostic_ping' });
  }

  async configureTemporary(target: TemporaryTarget): Promise<void> {
    const result = await this.request({ type: 'temporary_target', ...target }, 'temporary_result');
    if (result.code !== 'ok') throw new Error(temporaryFailure(result.code));
  }

  private request(
    value: { type: string; name?: string; host?: string; port?: number; kind?: string },
    expectedType: 'diagnostic_result' | 'temporary_result' = 'diagnostic_result',
  ): Promise<DiagnosticResult> {
    if (this.channel.readyState !== 'open') return Promise.reject(new Error('诊断通道不可用'));
    const id = randomControlID();
    return new Promise((resolve, reject) => {
      const timer = window.setTimeout(() => {
        this.pending.delete(id);
        reject(new Error('诊断请求超时'));
      }, 5_000);
      this.pending.set(id, { resolve, reject, timer, expectedType });
      this.channel.send(JSON.stringify({ ...value, id }));
    });
  }

  private receive(value: unknown) {
    if (typeof value !== 'string' || value.length > 512) {
      this.channel.close();
      return;
    }
    let response: (DiagnosticResult & { type?: string; id?: string }) | null = null;
    try {
      response = JSON.parse(value);
    } catch {
      this.channel.close();
      return;
    }
    if (
      (response?.type !== 'diagnostic_result' && response?.type !== 'temporary_result') ||
      !response.id
    ) {
      this.channel.close();
      return;
    }
    if (!['ok', 'blocked', 'unavailable', 'busy', 'invalid_request'].includes(response.code)) {
      this.channel.close();
      return;
    }
    const pending = this.pending.get(response.id);
    if (!pending) return;
    if (response.type !== pending.expectedType) {
      this.channel.close();
      return;
    }
    window.clearTimeout(pending.timer);
    this.pending.delete(response.id);
    pending.resolve(response);
  }

  private failAll(error: Error) {
    for (const pending of this.pending.values()) {
      window.clearTimeout(pending.timer);
      pending.reject(error);
    }
    this.pending.clear();
  }
}

function temporaryFailure(code: DiagnosticResult['code']): string {
  const messages: Record<DiagnosticResult['code'], string> = {
    ok: '',
    blocked: '临时目标被 OpenWrt Agent 安全策略拒绝',
    unavailable: 'OpenWrt Agent 无法验证该临时目标',
    busy: 'OpenWrt Agent 正在处理其他临时请求，请稍后重试',
    invalid_request: '临时目标配置无效或已被设置',
  };
  return messages[code];
}

function randomControlID(): string {
  const value = new Uint8Array(12);
  crypto.getRandomValues(value);
  return btoa(String.fromCharCode(...value))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

function textMessage(channel: RTCDataChannel, expected: string, timeoutMS: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => finish(new Error('DataChannel 鉴权超时')), timeoutMS);
    const message = (event: MessageEvent) => {
      if (event.data !== expected) finish(new Error('DataChannel 返回了无效鉴权消息'));
      else finish();
    };
    const close = () => finish(new Error('DataChannel 在鉴权期间关闭'));
    const finish = (error?: Error) => {
      window.clearTimeout(timer);
      channel.removeEventListener('message', message);
      channel.removeEventListener('close', close);
      if (error) reject(error);
      else resolve();
    };
    channel.addEventListener('message', message);
    channel.addEventListener('close', close, { once: true });
  });
}

function open(channel: RTCDataChannel): Promise<void> {
  if (channel.readyState === 'open') return Promise.resolve();
  return new Promise((resolve, reject) => {
    channel.addEventListener('open', () => resolve(), { once: true });
    channel.addEventListener('close', () => reject(new Error('DataChannel 未能建立')), {
      once: true,
    });
    channel.addEventListener('error', () => reject(new Error('DataChannel 建立失败')), {
      once: true,
    });
  });
}

function gathered(peer: RTCPeerConnection): Promise<void> {
  if (peer.iceGatheringState === 'complete') return Promise.resolve();
  return new Promise((resolve) => {
    const changed = () => {
      if (peer.iceGatheringState === 'complete') {
        peer.removeEventListener('icegatheringstatechange', changed);
        resolve();
      }
    };
    peer.addEventListener('icegatheringstatechange', changed);
  });
}

function connected(peer: RTCPeerConnection, timeoutMS: number): Promise<void> {
  if (peer.connectionState === 'connected') return Promise.resolve();
  return new Promise((resolve, reject) => {
    const timer = window.setTimeout(() => finish(new Error('P2P 连接超时')), timeoutMS);
    const changed = () => {
      if (peer.connectionState === 'connected') finish();
      else if (peer.connectionState === 'closed' || peer.connectionState === 'failed')
        finish(new Error('P2P 连接失败'));
    };
    const finish = (error?: Error) => {
      window.clearTimeout(timer);
      peer.removeEventListener('connectionstatechange', changed);
      if (error) reject(error);
      else resolve();
    };
    peer.addEventListener('connectionstatechange', changed);
  });
}

async function selectedPair(
  peer: RTCPeerConnection,
  gatheredTypes: ReadonlySet<string>,
): Promise<DirectStats> {
  const stats = await peer.getStats();
  let pair: RTCIceCandidatePairStats | undefined;
  stats.forEach((entry) => {
    if (
      entry.type === 'candidate-pair' &&
      entry.state === 'succeeded' &&
      (entry.nominated || entry.selected)
    ) {
      pair = entry as RTCIceCandidatePairStats;
    }
  });
  if (!pair) throw new Error('无法确认已选择的 P2P candidate pair');
  const local = stats.get(pair.localCandidateId) as CandidateStats | undefined;
  const remote = stats.get(pair.remoteCandidateId) as CandidateStats | undefined;
  if (
    !local ||
    !remote ||
    !allowedCandidateTypes.has(local.candidateType) ||
    !allowedCandidateTypes.has(remote.candidateType)
  ) {
    throw new Error('candidate pair 不满足无中继约束');
  }
  const protocol = String(local.protocol || remote.protocol || '').toLowerCase();
  if (protocol !== 'udp') throw new Error('当前连接不是 P2P UDP');
  const rtt = pair.currentRoundTripTime;
  return {
    localType: local.candidateType,
    remoteType: remote.candidateType,
    protocol: 'UDP',
    rttMS: typeof rtt === 'number' && Number.isFinite(rtt) ? Math.round(rtt * 1_000) : null,
    hostCandidate: gatheredTypes.has('host'),
    srflxCandidate: gatheredTypes.has('srflx'),
    dataChannels: 'open',
  };
}

function validateSTUN(values: readonly string[]) {
  if (!values.length || values.some((value) => !/^stun:[^@\s]+$/i.test(value))) {
    throw new Error('浏览器 ICE 配置只允许无凭据 stun: 地址');
  }
}

function rejectRelay(sdp: string) {
  if (/\btyp\s+relay\b/i.test(sdp)) throw new Error('连接描述包含被禁止的 relay candidate');
}

function failureMessage(code = 'agent_rejected'): string {
  const known: Record<string, string> = {
    invalid_offer: 'Agent 拒绝了无效连接描述',
    invalid_capability: '临时会话授权无效',
    service_blocked: 'Agent 未授权该服务',
    peer_setup_failed: 'Agent 无法创建 P2P 连接',
    restart_rejected: 'Agent 拒绝了 ICE 重启',
    negotiation_failed: 'P2P 协商失败',
  };
  return known[code] || 'Agent 拒绝了连接';
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function asError(value: unknown): Error {
  return value instanceof Error ? value : new Error(String(value));
}
