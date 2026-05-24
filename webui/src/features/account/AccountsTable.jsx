import { Check, Edit3, Plus, RefreshCw, Save, Star, Trash2 } from 'lucide-react'
import clsx from 'clsx'

import { modeLabel } from './providerMode'
import { maskSecret } from '../../utils/maskSecret'

export default function AccountsTable({
    t,
    providersConfig,
    onAddProvider,
    onEditProvider,
    onDeleteProvider,
    onSetActiveProvider,
    onRefreshProvider,
    refreshingProvider,
    savingProvider,
}) {
    const providers = providersConfig.providers || []
    const active = providersConfig.active || providers[0]?.id || ''

    return (
        <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
            <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-start justify-between gap-4">
                <div>
                    <h2 className="text-lg font-semibold">{t('providerManager.title')}</h2>
                    <p className="text-sm text-muted-foreground">{t('providerManager.description')}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                    <button
                        onClick={onRefreshProvider}
                        disabled={refreshingProvider}
                        className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-border bg-secondary text-secondary-foreground hover:bg-secondary/80 text-sm disabled:opacity-50"
                    >
                        <RefreshCw className={clsx('w-4 h-4', refreshingProvider && 'animate-spin')} />
                        {refreshingProvider ? t('actions.loading') : t('actions.refresh')}
                    </button>
                    <button
                        onClick={onAddProvider}
                        className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors font-medium text-sm shadow-sm"
                    >
                        <Plus className="w-4 h-4" />
                        {t('providerManager.addProvider')}
                    </button>
                </div>
            </div>

            <div className="divide-y divide-border">
                {providers.length > 0 ? providers.map(provider => {
                    const isActive = provider.id === active
                    return (
                        <div key={provider.id} className="p-4 flex flex-col lg:flex-row lg:items-center justify-between gap-4 hover:bg-muted/30 transition-colors">
                            <div className="min-w-0 flex-1 space-y-2">
                                <div className="flex flex-wrap items-center gap-2">
                                    <h3 className="font-medium text-foreground truncate">{provider.name || provider.base_url || provider.id}</h3>
                                    {isActive && (
                                        <span className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 text-emerald-600">
                                            <Check className="w-3 h-3" />
                                            {t('providerManager.activeBadge')}
                                        </span>
                                    )}
                                </div>
                                <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-2 text-xs text-muted-foreground">
                                    <div className="truncate"><span className="font-medium text-foreground/70">URL:</span> {provider.base_url || '-'}</div>
                                    <div className="truncate"><span className="font-medium text-foreground/70">{t('providerManager.modelLabel')}:</span> {provider.model || '-'}</div>
                                    <div><span className="font-medium text-foreground/70">{t('providerManager.modeLabel')}:</span> {modeLabel(t, provider.mode)}</div>
                                    <div><span className="font-medium text-foreground/70">{t('providerManager.apiKeyLabel')}:</span> {provider.api_key_preview || (provider.api_key ? maskSecret(provider.api_key) : '-')}</div>
                                </div>
                            </div>
                            <div className="flex flex-wrap items-center gap-2">
                                <button
                                    onClick={() => onSetActiveProvider(provider.id)}
                                    disabled={isActive || savingProvider}
                                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border bg-background hover:bg-secondary text-xs disabled:opacity-50"
                                >
                                    <Star className="w-3.5 h-3.5" />
                                    {t('providerManager.setActive')}
                                </button>
                                <button
                                    onClick={() => onEditProvider(provider)}
                                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border bg-background hover:bg-secondary text-xs"
                                >
                                    <Edit3 className="w-3.5 h-3.5" />
                                    {t('actions.edit')}
                                </button>
                                <button
                                    onClick={() => onDeleteProvider(provider.id)}
                                    disabled={savingProvider}
                                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border bg-background hover:bg-destructive/10 hover:text-destructive text-xs disabled:opacity-50"
                                >
                                    <Trash2 className="w-3.5 h-3.5" />
                                    {t('actions.delete')}
                                </button>
                            </div>
                        </div>
                    )
                }) : (
                    <div className="p-8 text-center text-muted-foreground">
                        <p className="font-medium text-foreground">{t('providerManager.emptyTitle')}</p>
                        <p className="text-sm mt-1">{t('providerManager.emptyDesc')}</p>
                    </div>
                )}
            </div>
        </div>
    )
}
