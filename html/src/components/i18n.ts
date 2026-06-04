// Re-export shim so component files can `import { t, Lang } from '../i18n'`
// regardless of their depth under `src/components/`.
export { t, getLang, setLang, DEFAULT_LANG, LANG_STORAGE_KEY, SUPPORTED_LANGS, type Lang } from '../i18n';
