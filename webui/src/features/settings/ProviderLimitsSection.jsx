export default function ProviderLimitsSection({ t, config, apiFetch, onRefresh, onMessage }) {
    const providers = config?.external_ai_providers?.providers || []
    if (!providers.length) return null

    const handleSave = async (providerId, maxInflight, maxQueue) => {
        const nextProviders = providers.map(p =>
            p.id === providerId ? { ...p, max_inflight: maxInflight, max_queue: maxQueue } : p
        )
        try {
            const res = await apiFetch('/admin/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    external_ai_providers: {
                        active: config.external_ai_providers?.active || '',
                        providers: nextProviders,
                    },
                }),
            })
            if (res.ok) {
                onMessage('success', t('providerManager.saveSuccess'))
                onRefresh()
            } else {
                const data = await res.json().catch(() => ({}))
                onMessage('error', data.detail || t('settings.saveFailed'))
            }
        } catch (_e) {
            onMessage('error', t('settings.saveFailed'))
        }
    }

    return (
        <div className="bg-card border border-border rounded-xl p-5 space-y-4">
            <h3 className="font-semibold">{t('settings.providerLimitsTitle')}</h3>
            <p className="text-sm text-muted-foreground">{t('settings.providerLimitsDesc')}</p>
            <div className="space-y-4">
                {providers.map(provider => (
                    <ProviderLimitRow
                        key={provider.id}
                        t={t}
                        provider={provider}
                        onSave={handleSave}
                    />
                ))}
            </div>
        </div>
    )
}

function ProviderLimitRow({ t, provider, onSave }) {
    return (
        <div className="rounded-lg border border-border bg-background/50 p-4">
            <div className="flex items-center justify-between mb-3">
                <span className="font-medium text-sm text-foreground">
                    {provider.name || provider.base_url || provider.id}
                </span>
            </div>
            <div className="grid grid-cols-2 gap-3">
                <label className="text-sm space-y-1.5">
                    <span className="text-muted-foreground text-xs">{t('providerManager.maxInflightShort')}</span>
                    <input
                        type="number"
                        min="0"
                        defaultValue={provider.max_inflight || 0}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm"
                        id={`${provider.id}-inflight`}
                    />
                </label>
                <label className="text-sm space-y-1.5">
                    <span className="text-muted-foreground text-xs">{t('providerManager.maxQueueShort')}</span>
                    <input
                        type="number"
                        min="0"
                        defaultValue={provider.max_queue || 0}
                        className="w-full bg-background border border-border rounded-lg px-3 py-2 text-sm"
                        id={`${provider.id}-queue`}
                    />
                </label>
            </div>
            <div className="flex justify-end mt-3">
                <button
                    onClick={() => {
                        const inflight = parseInt(document.getElementById(`${provider.id}-inflight`)?.value || '0', 10)
                        const queue = parseInt(document.getElementById(`${provider.id}-queue`)?.value || '0', 10)
                        onSave(provider.id, inflight, queue)
                    }}
                    className="px-3 py-1.5 text-xs rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
                >
                    {t('providerManager.save')}
                </button>
            </div>
        </div>
    )
}
