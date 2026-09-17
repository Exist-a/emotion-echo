// Type declarations for three/examples subpath imports.
// three's package.json exports field doesn't cover examples/jsm/*,
// so TypeScript with moduleResolution "Bundler" can't resolve them.
declare module 'three/examples/jsm/loaders/GLTFLoader' {
  import type { Object3D, LoadingManager } from 'three'

  export class GLTFParser {
    json: Record<string, unknown>
    plugins: Record<string, unknown>
    extensions: Record<string, unknown>
  }

  export interface GLTF {
    scene: Object3D
    scenes: Object3D[]
    cameras: unknown[]
    animations: unknown[]
    asset: { version: string; generator: string }
    userData: Record<string, unknown>
  }

  export class GLTFLoader {
    constructor(manager?: LoadingManager)
    register(callback: (parser: GLTFParser) => unknown): this
    unregister(callback: (parser: GLTFParser) => unknown): this
    loadAsync(url: string, onProgress?: (event: ProgressEvent) => void): Promise<GLTF>
  }
}
