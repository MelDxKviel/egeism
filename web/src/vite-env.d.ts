/// <reference types="vite/client" />

interface ImportMetaEnv {
  // Optional API origin. Empty (default) = same-origin / dev proxy.
  readonly VITE_API_BASE?: string;
}
interface ImportMeta {
  readonly env: ImportMetaEnv;
}
