export const PROVIDER_MODES = ['auto', 'openai', 'claude', 'gemini']

export function normalizeProviderMode(mode) {
    const value = String(mode || '').trim().toLowerCase()
    return PROVIDER_MODES.includes(value) ? value : 'auto'
}

export function detectProviderMode(baseURL, apiKey) {
    const url = String(baseURL || '').trim().toLowerCase()
    const key = String(apiKey || '').trim().toLowerCase()

    if (url.includes('anthropic') || url.includes('claude') || key.startsWith('sk-ant-')) {
        return 'claude'
    }
    if (url.includes('generativelanguage.googleapis.com') || url.includes('gemini') || key.startsWith('aiza')) {
        return 'gemini'
    }
    if (url.includes('/v1') || url.includes('openai') || url.includes('chat/completions') || key.startsWith('sk-')) {
        return 'openai'
    }
    return 'auto'
}

export function modeLabel(t, mode) {
    return t(`providerManager.modes.${normalizeProviderMode(mode)}`)
}
