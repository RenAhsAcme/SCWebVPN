import { Controller, type Frame } from '@mercuryworkshop/scramjet-controller';
import type { ProxyTransport } from '@mercuryworkshop/proxy-transports';

let controller: Controller | null = null;
let frame: Frame | null = null;

export async function loadRemote(
  transport: ProxyTransport,
  element: HTMLIFrameElement,
  remote: URL,
): Promise<void> {
  if (!controller) {
    controller = new Controller({
      serviceworker: await activeServiceWorker(),
      transport,
      config: {
        scramjetPath: '/compat/scramjet/scramjet.js',
        injectPath: '/compat/controller/controller.inject.js',
        wasmPath: '/compat/scramjet/scramjet.wasm',
      },
    });
    await controller.wait();
  } else {
    controller.setTransport(transport);
  }
  frame = controller.createFrame(element);
  frame.go(remote.href);
}

async function activeServiceWorker(): Promise<ServiceWorker> {
  await navigator.serviceWorker.register('/controller.sw.js', { scope: '/' });
  const registration = await navigator.serviceWorker.ready;
  if (!registration.active) throw new Error('浏览器兼容服务未就绪');
  return registration.active;
}
