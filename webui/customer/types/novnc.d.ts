declare module "@novnc/novnc/lib/rfb" {
  export interface RFBOptions {
    credentials?: {
      password?: string
      username?: string
      target?: string
    }
    wsProtocols?: string[]
  }

  export interface RFBEvents {
    connect: CustomEvent
    disconnect: CustomEvent<{ clean: boolean }>
    credentialsrequired: CustomEvent<{ types: string[] }>
    securityfailure: CustomEvent<{ reason: string }>
    desktopname: CustomEvent<{ name: string }>
    capabilities: CustomEvent<{ capabilities: string[] }>
  }

  export default class RFB {
    constructor(
      target: HTMLElement,
      url: string,
      options?: RFBOptions
    )

    disconnect(): void
    sendCredentials(credentials: { password?: string; username?: string }): void
    clipboardPasteFrom(text: string): void
    sendCtrlAltDel(): void
    scaleViewport: boolean
    resizeSession: boolean
    clipViewport: boolean
    showDotCursor: boolean

    addEventListener<K extends keyof RFBEvents>(
      type: K,
      listener: (event: RFBEvents[K]) => void
    ): void

    removeEventListener<K extends keyof RFBEvents>(
      type: K,
      listener: (event: RFBEvents[K]) => void
    ): void
  }
}