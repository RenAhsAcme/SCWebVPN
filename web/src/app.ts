import { RTCEpoxyTransport } from './compat';
import {
  browserConfig,
  createBindingCode,
  createTemporaryService,
  currentTemporaryService,
  DirectConnection,
  type ControlClient,
  agentOnline,
  type DirectStats,
  services as loadServices,
  type TemporaryTarget,
} from './connection';
import { loadRemote } from './scramjet';
import { virtualHostname } from './virtual';

type Service = {
  id: string;
  agent_id: string;
  slug: string;
  display_name: string;
  kind: 'http' | 'https' | 'guacamole';
  enabled: boolean;
  temporary?: boolean;
};

const elements = {
  services: required<HTMLElement>('services'),
  status: required<HTMLOutputElement>('status'),
  details: required<HTMLElement>('details'),
  frame: required<HTMLIFrameElement>('remote'),
  restart: required<HTMLButtonElement>('restart'),
  disconnect: required<HTMLButtonElement>('disconnect'),
  logout: required<HTMLButtonElement>('logout'),
  bindingPanel: required<HTMLElement>('agent-binding'),
  bindingCreate: required<HTMLButtonElement>('binding-create'),
  bindingResult: required<HTMLElement>('binding-result'),
  bindingCode: required<HTMLOutputElement>('binding-code'),
  bindingExpiry: required<HTMLElement>('binding-expiry'),
  dnsName: required<HTMLInputElement>('dns-name'),
  dns: required<HTMLButtonElement>('dns-query'),
  ping: required<HTMLButtonElement>('ping-target'),
  diagnostic: required<HTMLOutputElement>('diagnostic-result'),
  temporaryCreatePanel: required<HTMLElement>('temporary-create'),
  temporaryAgent: required<HTMLSelectElement>('temporary-agent'),
  temporaryKind: required<HTMLSelectElement>('temporary-kind'),
  temporaryCreate: required<HTMLButtonElement>('temporary-create-button'),
  temporaryTargetPanel: required<HTMLElement>('temporary-target'),
  temporaryURL: required<HTMLInputElement>('temporary-url'),
  temporaryConnect: required<HTMLButtonElement>('temporary-connect'),
};

let direct: DirectConnection | null = null;
let activeService: Service | null = null;
let control: ControlClient | null = null;
let temporaryService: Service | null = null;
let temporarySTUNURLs: string[] = [];
let bindingExpiryTimer: number | undefined;

void initialize();

async function initialize() {
  if (!('RTCPeerConnection' in window) || !('serviceWorker' in navigator)) {
    fail(new Error('当前浏览器不支持所需的 WebRTC 或 Service Worker 能力'));
    return;
  }
  try {
    const requested = requestedSlug();
    elements.bindingPanel.hidden = requested !== null;
    if (requested?.startsWith('tmp-')) {
      const [config, service] = await Promise.all([
        browserConfig(),
        currentTemporaryService<Service>(),
      ]);
      if (!service.temporary || service.slug !== requested) throw new Error('临时入口已失效');
      temporaryService = service;
      temporarySTUNURLs = config.stun_urls;
      elements.services.closest<HTMLElement>('.panel')!.hidden = true;
      elements.temporaryCreatePanel.hidden = true;
      elements.temporaryTargetPanel.hidden = false;
      elements.temporaryURL.placeholder = `${service.kind}://内网主机:${service.kind === 'https' ? '443' : '80'}/`;
      setStatus('临时入口已就绪；目标尚未发送给 OpenWrt Agent。');
      return;
    }
    const [config, catalog] = await Promise.all([browserConfig(), loadServices<Service>()]);
    renderServices(catalog);
    renderTemporaryAgents(catalog);
    const selected = requested ? catalog.find((service) => service.slug === requested) : undefined;
    if (requested && !selected) throw new Error('该服务不存在或当前不可用');
    if (selected) await connect(selected, config.stun_urls);
  } catch (error) {
    if (requestedSlug()?.startsWith('tmp-')) void clearTemporaryOrigin();
    fail(asError(error));
  }
}

function renderTemporaryAgents(catalog: Service[]) {
  elements.temporaryAgent.replaceChildren();
  const agents = [...new Set(catalog.map((service) => service.agent_id))];
  agents.forEach((agentID, index) => {
    const option = document.createElement('option');
    option.value = agentID;
    option.textContent = agents.length === 1 ? 'OpenWrt Agent' : `OpenWrt Agent ${index + 1}`;
    elements.temporaryAgent.append(option);
  });
  elements.temporaryCreate.disabled = agents.length === 0;
}

function renderServices(catalog: Service[]) {
  elements.services.replaceChildren();
  for (const service of catalog) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'service';
    const name = document.createElement('strong');
    const kind = document.createElement('span');
    name.textContent = service.display_name;
    kind.textContent = service.kind === 'guacamole' ? '远程桌面与 SSH' : service.kind.toUpperCase();
    button.append(name, kind);
    button.addEventListener('click', () => void connectFromPortal(service));
    elements.services.append(button);
  }
  if (!catalog.length) elements.services.textContent = '尚未配置可访问的内网服务。';
}

async function connectFromPortal(service: Service) {
  const config = await browserConfig();
  await connect(service, config.stun_urls);
}

async function connect(
  service: Service,
  stunURLs: string[],
  temporary?: { target: TemporaryTarget; remote: URL },
) {
  direct?.close();
  activeService = service;
  elements.frame.hidden = true;
  elements.restart.disabled = true;
  elements.disconnect.disabled = false;
  setStatus(`正在连接 ${service.display_name}…`);
  if (!(await agentOnline(service.agent_id))) {
    failP2P(new Error('OpenWrt Agent 当前离线'));
    return;
  }
  direct = new DirectConnection(stunURLs, {
    status: setStatus,
    stats: renderStats,
    failed: failP2P,
  });
  try {
    const session = await direct.connect(service.id, temporary?.target);
    control = session.control;
    setDiagnosticsEnabled(true);
    const transport = new RTCEpoxyTransport(session.wisp);
    await transport.init();
    await loadRemote(transport, elements.frame, virtualURL(service, temporary?.remote));
    elements.frame.hidden = false;
    elements.restart.disabled = false;
  } catch (error) {
    failP2P(asError(error));
  }
}

function virtualURL(service: Service, remote?: URL): URL {
  const scheme = service.kind === 'https' ? 'https:' : 'http:';
  const result = new URL(`${scheme}//${virtualHostname(service.id)}/`);
  if (remote) {
    result.pathname = remote.pathname;
    result.search = remote.search;
    result.hash = remote.hash;
  }
  return result;
}

function renderStats(stats: DirectStats | null) {
  elements.details.replaceChildren();
  if (!stats) return;
  const values = [
    ['host candidate', stats.hostCandidate ? '已获得' : '未获得'],
    ['srflx candidate', stats.srflxCandidate ? '已获得' : '未获得'],
    ['本地 candidate', stats.localType],
    ['远端 candidate', stats.remoteType],
    ['传输', stats.protocol],
    ['往返延迟', stats.rttMS == null ? '暂不可用' : `${stats.rttMS} ms`],
    ['DataChannel', stats.dataChannels],
    ['丢包', 'RTCDataChannel 无独立统计'],
  ];
  for (const [name, value] of values) {
    const term = document.createElement('dt');
    const description = document.createElement('dd');
    term.textContent = name;
    description.textContent = value;
    elements.details.append(term, description);
  }
}

function failP2P(error: Error) {
  direct?.close();
  control = null;
  setDiagnosticsEnabled(false);
  elements.frame.hidden = true;
  elements.restart.disabled = true;
  elements.disconnect.disabled = true;
  elements.status.textContent =
    '当前网络无法建立点对点连接。\n\n' +
    '可能原因：对称 NAT、运营商 CGNAT 限制、UDP 被阻断，或企业、校园、公共网络防火墙限制。\n\n' +
    '可尝试：切换至其他网络；使用手机热点；检查 OpenWrt 出站防火墙；使用现有 RustDesk 备用入口。\n\n' +
    '连接已明确终止；系统不会尝试 TURN、relay 或服务器中继。\n\n' +
    `详情：${error.message}`;
}

function fail(error: Error) {
  setStatus(`无法初始化 WebVPN：${error.message}`);
}

function setStatus(message: string) {
  elements.status.textContent = message;
}

function requestedSlug(): string | null {
  const suffix = '.vpn.example.com';
  const host = location.hostname.toLowerCase();
  if (!host.endsWith(suffix)) return null;
  const prefix = host.slice(0, -suffix.length);
  return prefix && !prefix.includes('.') ? prefix : null;
}

elements.restart.addEventListener('click', () => void direct?.restart());
elements.bindingCreate.addEventListener('click', async () => {
  clearBindingCode();
  elements.bindingCreate.disabled = true;
  try {
    const issued = await createBindingCode();
    elements.bindingCode.textContent = issued.code;
    elements.bindingExpiry.textContent = `有效至 ${new Date(issued.expiresAt).toLocaleTimeString(
      [],
      {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      },
    )}`;
    elements.bindingResult.hidden = false;
    bindingExpiryTimer = window.setTimeout(clearBindingCode, issued.expiresAt - Date.now());
  } catch (error) {
    elements.bindingExpiry.textContent = asError(error).message;
    elements.bindingResult.hidden = false;
    elements.bindingCreate.disabled = false;
  }
});
elements.disconnect.addEventListener('click', () => {
  direct?.close();
  direct = null;
  activeService = null;
  control = null;
  setDiagnosticsEnabled(false);
  elements.frame.hidden = true;
  elements.restart.disabled = true;
  elements.disconnect.disabled = true;
  setStatus('已断开。');
});
elements.dns.addEventListener('click', async () => {
  const name = elements.dnsName.value.trim();
  if (!control || !name) return;
  elements.diagnostic.textContent = '正在通过 Agent 解析…';
  try {
    const result = await control.dns(name);
    elements.diagnostic.textContent =
      result.code === 'ok'
        ? `DNS：${result.addresses?.join(', ') || '无结果'}`
        : `DNS 诊断：${diagnosticMessage(result.code)}`;
  } catch (error) {
    elements.diagnostic.textContent = asError(error).message;
  }
});
elements.ping.addEventListener('click', async () => {
  if (!control) return;
  elements.diagnostic.textContent = '正在 Ping 当前授权服务…';
  try {
    const result = await control.ping();
    elements.diagnostic.textContent =
      result.code === 'ok'
        ? `Ping：${result.rtt_ms ?? 0} ms`
        : `Ping 诊断：${diagnosticMessage(result.code)}`;
  } catch (error) {
    elements.diagnostic.textContent = asError(error).message;
  }
});
elements.logout.addEventListener('click', async () => {
  direct?.close();
  clearBindingCode();
  await fetch('/api/v1/browser/logout', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
  if (requestedSlug()?.startsWith('tmp-')) await clearTemporaryOrigin();
  location.assign('/auth/logout');
});
elements.temporaryCreate.addEventListener('click', async () => {
  elements.temporaryCreate.disabled = true;
  try {
    const kind = elements.temporaryKind.value === 'http' ? 'http' : 'https';
    const created = await createTemporaryService<Service>({
      agent_id: elements.temporaryAgent.value,
      display_name: kind === 'https' ? '临时 HTTPS 服务' : '临时 HTTP 服务',
      kind,
    });
    location.assign(`https://${created.slug}.${location.hostname}/`);
  } catch (error) {
    fail(asError(error));
    elements.temporaryCreate.disabled = false;
  }
});
elements.temporaryConnect.addEventListener('click', async () => {
  if (!temporaryService) return;
  try {
    const remote = parseTemporaryURL(elements.temporaryURL.value, temporaryService.kind);
    elements.temporaryConnect.disabled = true;
    await connect(temporaryService, temporarySTUNURLs, {
      remote,
      target: {
        host: stripIPv6Brackets(remote.hostname),
        port: Number(remote.port || (remote.protocol === 'https:' ? 443 : 80)),
        kind: remote.protocol === 'https:' ? 'https' : 'http',
      },
    });
  } catch (error) {
    failP2P(asError(error));
    elements.temporaryConnect.disabled = false;
  }
});
window.addEventListener('pagehide', clearBindingCode);
window.addEventListener('beforeunload', () => direct?.close());

function required<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) throw new Error(`missing UI element: ${id}`);
  return element as T;
}

function asError(value: unknown): Error {
  return value instanceof Error ? value : new Error(String(value));
}

function setDiagnosticsEnabled(enabled: boolean) {
  elements.dns.disabled = !enabled;
  elements.ping.disabled = !enabled;
}

function clearBindingCode() {
  if (bindingExpiryTimer !== undefined) window.clearTimeout(bindingExpiryTimer);
  bindingExpiryTimer = undefined;
  elements.bindingCode.textContent = '';
  elements.bindingExpiry.textContent = '';
  elements.bindingResult.hidden = true;
  elements.bindingCreate.disabled = false;
}

function diagnosticMessage(code: string): string {
  const messages: Record<string, string> = {
    blocked: '目标不在当前服务的允许范围内',
    unavailable: 'Agent 当前无法完成该诊断',
    busy: '诊断繁忙，请稍后重试',
    invalid_request: '请求格式无效',
  };
  return messages[code] || '未知结果';
}

function parseTemporaryURL(value: string, expectedKind: Service['kind']): URL {
  let target: URL;
  try {
    target = new URL(value.trim());
  } catch {
    throw new Error('请输入完整的 HTTP 或 HTTPS 内网 URL');
  }
  const kind = target.protocol === 'https:' ? 'https' : target.protocol === 'http:' ? 'http' : null;
  if (!kind || kind !== expectedKind || target.username || target.password || !target.hostname) {
    throw new Error(`该临时入口只接受不含凭据的 ${expectedKind.toUpperCase()} URL`);
  }
  const port = Number(target.port || (kind === 'https' ? 443 : 80));
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('目标端口无效');
  return target;
}

function stripIPv6Brackets(host: string): string {
  return host.startsWith('[') && host.endsWith(']') ? host.slice(1, -1) : host;
}

async function clearTemporaryOrigin() {
  const registrations = await navigator.serviceWorker?.getRegistrations().catch(() => []);
  await Promise.all((registrations || []).map((registration) => registration.unregister()));
  if ('caches' in window) {
    const names = await caches.keys().catch(() => []);
    await Promise.all(names.map((name) => caches.delete(name)));
  }
}
