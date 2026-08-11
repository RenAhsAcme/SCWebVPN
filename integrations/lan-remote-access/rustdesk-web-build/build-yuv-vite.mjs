import { build } from 'vite';

await build({
  configFile: false,
  root: process.cwd(),
  build: {
    commonjsOptions: { include: [/yuv-src/, /build/] },
    emptyOutDir: false,
    lib: {
      entry: 'yuv-src/yuv-canvas.js',
      name: 'YUVCanvas',
      formats: ['iife'],
      fileName: () => 'yuv-canvas-1.2.6.js',
    },
    outDir: 'runtime',
  },
});
