import { existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';

const root = import.meta.dir.replace(/[\\/]scripts$/, '');
const output = join(root, 'dist');
const modules = join(root, 'node_modules');
const epoxyVendor = join(root, 'vendor', 'epoxy');
const epoxyModule = join(epoxyVendor, 'epoxy_client.js');
const epoxyWasm = join(epoxyVendor, 'epoxy_client_bg.wasm');
const controllerModule = join(
  modules,
  '@mercuryworkshop',
  'scramjet-controller',
  'dist',
  'controller-external.mjs',
);
if (!existsSync(epoxyModule) || !existsSync(epoxyWasm)) {
  throw new Error('Pinned Epoxy source artifact is missing from web/vendor/epoxy.');
}
mkdirSync(join(output, 'compat', 'controller'), { recursive: true });
mkdirSync(join(output, 'compat', 'scramjet'), { recursive: true });

const build = await Bun.build({
  entrypoints: [join(root, 'src', 'app.ts')],
  minify: true,
  outdir: output,
  target: 'browser',
  format: 'esm',
  sourcemap: 'none',
  plugins: [
    {
      name: 'pinned-epoxy',
      setup(builder) {
        builder.onResolve({ filter: /^@mercuryworkshop\/epoxy-tls$/ }, () => ({
          path: epoxyModule,
        }));
        builder.onResolve({ filter: /^@mercuryworkshop\/scramjet-controller$/ }, () => ({
          path: controllerModule,
        }));
      },
    },
  ],
});
if (!build.success) {
  for (const message of build.logs) console.error(message);
  process.exit(1);
}

const copies = [
  ['public/index.html', 'index.html'],
  ['public/style.css', 'style.css'],
  [epoxyWasm, 'epoxy_client_bg.wasm'],
  [join(modules, '@mercuryworkshop/scramjet/dist/scramjet.js'), 'compat/scramjet/scramjet.js'],
  [join(modules, '@mercuryworkshop/scramjet/dist/scramjet.wasm'), 'compat/scramjet/scramjet.wasm'],
  [
    join(modules, '@mercuryworkshop/scramjet-controller/dist/controller.inject.js'),
    'compat/controller/controller.inject.js',
  ],
  [
    join(modules, '@mercuryworkshop/scramjet-controller/dist/controller.api.js'),
    'compat/controller/controller.api.js',
  ],
  [
    join(modules, '@mercuryworkshop/scramjet-controller/dist/controller.sw.js'),
    'compat/controller/controller.lib.js',
  ],
  ['src/controller-sw.js', 'controller.sw.js'],
] as const;
for (const [source, destination] of copies) {
  const absolute = source.includes(':') ? source : join(root, source);
  if (!existsSync(absolute)) throw new Error(`Required build input is missing: ${source}`);
  await Bun.write(join(output, destination), Bun.file(absolute));
}
