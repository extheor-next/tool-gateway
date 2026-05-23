import { X } from 'lucide-react'

import { modeLabel, PROVIDER_MODES } from './providerMode'

export default function AddAccountModal({
    show,
    t,
    provider,
    editingProvider,
    onProviderChange,
    loading,
    onClose,
    onAdd,
}) {
    if (!show) {
        return null
    }

    const isEditing = Boolean(editingProvider?.id)
    const hasStoredKey = Boolean(editingProvider?.has_api_key || editingProvider?.api_key_preview)

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-in fade-in">
            <div className="bg-card w-full max-w-lg rounded-xl border border-border shadow-2xl overflow-hidden animate-in zoom-in-95">
                <div className="p-4 border-b border-border flex justify-between items-center">
                    <h3 className="font-semibold">{isEditing ? t('providerManager.editProvider') : t('providerManager.addProvider')}</h3>
                    <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
                        <X className="w-5 h-5" />
                    </button>
                </div>
                <div className="p-6 space-y-4">
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('providerManager.nameLabel')}</label>
                        <input
                            type="text"
                            className="input-field"
                            placeholder={t('providerManager.namePlaceholder')}
                            value={provider.name}
                            onChange={e => onProviderChange('name', e.target.value)}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('providerManager.urlLabel')} <span className="text-destructive">*</span></label>
                        <input
                            type="url"
                            className="input-field"
                            placeholder="https://api.openai.com/v1"
                            value={provider.base_url}
                            onChange={e => onProviderChange('base_url', e.target.value)}
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium mb-1.5">{t('providerManager.apiKeyLabel')}</label>
                        <input
                            type="password"
                            className="input-field font-mono"
                            placeholder={hasStoredKey ? t('providerManager.keepKeyPlaceholder', { preview: editingProvider.api_key_preview }) : 'sk-...'}
                            value={provider.api_key}
                            onChange={e => onProviderChange('api_key', e.target.value)}
                        />
                        {hasStoredKey && <p className="text-xs text-muted-foreground mt-1.5">{t('providerManager.keepKeyHint')}</p>}
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('providerManager.modeLabel')}</label>
                            <select
                                className="input-field"
                                value={provider.mode}
                                onChange={e => onProviderChange('mode', e.target.value)}
                            >
                                {PROVIDER_MODES.map(mode => (
                                    <option key={mode} value={mode}>{modeLabel(t, mode)}</option>
                                ))}
                            </select>
                        </div>
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('providerManager.modelLabel')}</label>
                            <input
                                type="text"
                                className="input-field"
                                placeholder="gpt-4o-mini"
                                value={provider.model}
                                onChange={e => onProviderChange('model', e.target.value)}
                            />
                        </div>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('providerManager.maxInflightLabel')}</label>
                            <input
                                type="number"
                                min="0"
                                className="input-field"
                                value={provider.max_inflight}
                                onChange={e => onProviderChange('max_inflight', e.target.value)}
                            />
                            <p className="text-xs text-muted-foreground mt-1.5">{t('providerManager.maxInflightHelp')}</p>
                        </div>
                        <div>
                            <label className="block text-sm font-medium mb-1.5">{t('providerManager.maxQueueLabel')}</label>
                            <input
                                type="number"
                                min="0"
                                className="input-field"
                                value={provider.max_queue}
                                onChange={e => onProviderChange('max_queue', e.target.value)}
                            />
                            <p className="text-xs text-muted-foreground mt-1.5">{t('providerManager.maxQueueHelp')}</p>
                        </div>
                    </div>
                    <div className="flex justify-end gap-2 pt-2">
                        <button onClick={onClose} className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium">{t('actions.cancel')}</button>
                        <button onClick={onAdd} disabled={loading} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50">
                            {loading ? t('providerManager.saving') : t('providerManager.save')}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    )
}
