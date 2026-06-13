const path = require('path');
const fs = require('fs');
const net = require('net');
const { execSync } = require('child_process');
const { merge } = require('webpack-merge');
const webpack = require('webpack');
const ESLintPlugin = require('eslint-webpack-plugin');
const CopyWebpackPlugin = require('copy-webpack-plugin');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const CssMinimizerPlugin = require('css-minimizer-webpack-plugin');
const TerserPlugin = require('terser-webpack-plugin');

const devMode = process.env.NODE_ENV !== 'production';

function getBackendPort() {
    try {
        const daemonPath = path.join(process.env.HOME, '.1agents', 'daemon.json');
        const config = JSON.parse(fs.readFileSync(daemonPath, 'utf8'));
        const match = config.listen_addr.match(/:(\d+)$/);
        return match ? match[1] : '8080';
    } catch {
        return '8080';
    }
}

const backendPort = getBackendPort();

// Find the first free port at or above `startPort` so `yarn start` never
// crashes with EADDRINUSE — it just rolls forward to the next open port.
function findFreePort(startPort, maxTries = 50) {
    return new Promise((resolve, reject) => {
        let port = startPort;
        let tries = 0;
        const tryPort = () => {
            const tester = net
                .createServer()
                .once('error', err => {
                    if (err.code === 'EADDRINUSE' && tries < maxTries) {
                        tries++;
                        port++;
                        tryPort();
                    } else {
                        reject(err);
                    }
                })
                .once('listening', () => tester.once('close', () => resolve(port)).close())
                .listen(port, '0.0.0.0');
        };
        tryPort();
    });
}

// ── Build-time version metadata (consumed by html/src/version.ts) ────────────
// Order of precedence:
//   1. Environment variables (CI passes these explicitly).
//   2. `git describe` / `git rev-parse` (local dev).
//   3. Hard-coded fallback (sandbox / no git available).
function safeExec(cmd) {
    try {
        return execSync(cmd, { stdio: ['ignore', 'pipe', 'ignore'] })
            .toString()
            .trim();
    } catch {
        return '';
    }
}

const buildMeta = {
    version: process.env.APP_VERSION || safeExec('git describe --tags --always --dirty') || 'dev',
    commit: process.env.GIT_COMMIT || safeExec('git rev-parse --short HEAD') || 'none',
    buildTime: process.env.BUILD_TIME || new Date().toISOString(),
};

const baseConfig = {
    context: path.resolve(__dirname, 'src'),
    entry: {
        app: './index.tsx',
    },
    output: {
        path: path.resolve(__dirname, 'dist'),
        filename: devMode ? '[name].js' : '[name].[contenthash].js',
    },
    module: {
        rules: [
            {
                test: /\.tsx?$/,
                use: 'ts-loader',
                exclude: /node_modules/,
            },
            {
                test: /\.s?[ac]ss$/,
                use: [devMode ? 'style-loader' : MiniCssExtractPlugin.loader, 'css-loader', 'sass-loader'],
            },
            {
                test: /\.svg$/,
                type: 'asset/inline',
            },
        ],
    },
    resolve: {
        extensions: ['.tsx', '.ts', '.js'],
    },
    plugins: [
        new webpack.DefinePlugin({
            IS_DESKTOP: JSON.stringify(process.env.IS_DESKTOP === 'true'),
            __APP_VERSION__: JSON.stringify(buildMeta.version),
            __GIT_COMMIT__: JSON.stringify(buildMeta.commit),
            __BUILD_TIME__: JSON.stringify(buildMeta.buildTime),
        }),
        new ESLintPlugin({
            context: path.resolve(__dirname, '.'),
            extensions: ['js', 'jsx', 'ts', 'tsx'],
            cache: false,
        }),
        new CopyWebpackPlugin({
            patterns: [
                { from: './favicon.png', to: '.' },
                { from: './logo.png', to: '.' },
                { from: './manifest.json', to: '.' },
                { from: './sw.js', to: '.' },
                { from: './pwa-192.png', to: '.' },
                { from: './pwa-512.png', to: '.' },
                { from: './apple-touch-icon.png', to: '.' },
            ],
        }),
        new MiniCssExtractPlugin({
            filename: devMode ? '[name].css' : '[name].[contenthash].css',
            chunkFilename: devMode ? '[id].css' : '[id].[contenthash].css',
        }),
        new HtmlWebpackPlugin({
            inject: false,
            minify: {
                removeComments: true,
                collapseWhitespace: true,
            },
            title: 'ttyd - Terminal',
            template: './template.html',
        }),
    ],
    performance: {
        hints: false,
    },
};

const devConfig = {
    mode: 'development',
    devServer: {
        host: '0.0.0.0',
        static: path.join(__dirname, 'dist'),
        compress: true,
        port: 9000,
        hot: false,
        liveReload: false,
        client: false,
        webSocketServer: false,
        proxy: [
            {
                // WebSocket endpoints (Terminal and Bridge) — proxy to the Go backend
                context: ['/token', '/ws', '/bridge'],
                target: `http://localhost:${backendPort}`,
                ws: true,
                changeOrigin: true,
                headers: {
                    Origin: `http://localhost:${backendPort}`,
                },
            },
            {
                // HTTP API & Asset endpoints — proxy to the Go backend.
                // ws: true so WS upgrades under /api (e.g. /api/agent/chat/ws)
                // survive the dev proxy.
                context: ['/api', '/cc-connect', '/assets', '/1skills'],
                target: `http://localhost:${backendPort}`,
                changeOrigin: true,
                ws: true,
            },
            {
                // Submodule embed bundles — proxy to the Go backend in dev
                context: ['/api/embed'],
                target: `http://localhost:${backendPort}`,
                changeOrigin: true,
            },
        ],
    },
    devtool: 'inline-source-map',
};

const prodConfig = {
    mode: 'production',
    optimization: {
        minimizer: [new TerserPlugin(), new CssMinimizerPlugin()],
    },
    devtool: 'source-map',
};

module.exports = async () => {
    if (!devMode) return merge(baseConfig, prodConfig);
    // `port` in devConfig is the preferred base; roll forward if it's taken.
    const basePort = Number(process.env.PORT) || devConfig.devServer.port;
    const port = await findFreePort(basePort);
    if (port !== basePort) {
        console.log(`[webpack-dev-server] port ${basePort} busy → using ${port}`);
    }
    return merge(baseConfig, devConfig, { devServer: { port } });
};
