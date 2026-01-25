import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
    root: '.',
    publicDir: 'public',
    build: {
        outDir: 'dist/client',
        emptyOutDir: true,
        rollupOptions: {
            input: {
                main: resolve(__dirname, 'src/client/main.ts'),
            },
            output: {
                entryFileNames: 'assets/[name].[hash].js',
                chunkFileNames: 'assets/[name].[hash].js',
                assetFileNames: 'assets/[name].[hash].[ext]',
            },
        },
        sourcemap: true,
    },
    server: {
        port: 3000,
        strictPort: true,
    },
    resolve: {
        alias: {
            '@': resolve(__dirname, 'src'),
        },
    },
});
