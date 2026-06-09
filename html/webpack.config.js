const path = require('path');
const fs = require('fs');
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
        ],
    },
    resolve: {
        extensions: ['.tsx', '.ts', '.js'],
    },
    plugins: [
        new webpack.DefinePlugin({
            IS_DESKTOP: JSON.stringify(process.env.IS_DESKTOP === 'true'),
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
                // HTTP API & Asset endpoints — proxy to the Go backend
                context: ['/api', '/cc-connect', '/assets', '/1skills'],
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

module.exports = merge(baseConfig, devMode ? devConfig : prodConfig);
