declare const IS_DESKTOP: boolean;
declare const __APP_VERSION__: string;
declare const __GIT_COMMIT__: string;
declare const __BUILD_TIME__: string;

declare module '*.svg' {
    const content: string;
    export default content;
}
