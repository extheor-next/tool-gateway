import { useMemo, useState } from 'react'

import { useI18n } from '../../i18n'
import { useAccountActions } from './useAccountActions'
import ApiKeysPanel from './ApiKeysPanel'
import AccountsTable from './AccountsTable'
import AddKeyModal from './AddKeyModal'

function providerToForm(provider) {
    const headersText = Object.entries(provider?.headers || {}).map(([k, v]) => `${k}: ${v}`).join('\n')
    return {
        id: provider?.id || '',
        name: provider?.name || '',
        base_url: provider?.base_url || '',
        api_key: '',
        model: provider?.model || '',
        mode: provider?.mode || 'auto',
        max_inflight: provider?.max_inflight || 0,
        max_queue: provider?.max_queue || 0,
        headersText,
    }
}

function formToProvider(form, prev = {}) {
    const headers = {}
    String(form.headersText || '').split('\n').forEach(line => {
        const idx = line.indexOf(':')
        if (idx < 0) return
        const key = line.slice(0, idx).trim()
        const value = line.slice(idx + 1).trim()
        if (key && value) headers[key] = value
    })
    const next = {
        id: form.id || prev.id || '',
        name: form.name,
        base_url: form.base_url,
        api_key: form.api_key || (prev.has_api_key ? '' : ''),
        model: form.model,
        mode: form.mode,
        max_inflight: Number(form.max_inflight) || 0,
        max_queue: Number(form.max_queue) || 0,
        headers,
    }
    if (!form.api_key && prev.api_key_preview) {
        next.api_key = prev.api_key || ''
    }
    return next
}

export default function AccountManagerContainer({ config, onRefresh, onMessage, authFetch }) {
    const { t } = useI18n()
    const apiFetch = authFetch || fetch
    const [providerModalOpen, setProviderModalOpen] = useState(false)
    const [editingProvider, setEditingProvider] = useState(null)
    const [providerForm, setProviderForm] = useState(providerToForm(null))

    const {
        showAddKey,
        openAddKey,
        openEditKey,
        closeKeyModal,
        editingKey,
        newKey,
        setNewKey,
        copiedKey,
        setCopiedKey,
        provider,
        setProviderField,
        refreshProviderFromConfig,
        saveProvider,
        savingProvider,
        refreshingProvider,
        loading,
        addKey,
        deleteKey,
    } = useAccountActions({
        apiFetch,
        t,
        onMessage,
        onRefresh,
        config,
    })

    const providersConfig = config.external_ai_providers || { active: '', providers: [] }
    const providerCount = providersConfig.providers?.length || 0

    const openAddProvider = () => {
        setEditingProvider(null)
        setProviderForm(providerToForm(null))
        setProviderModalOpen(true)
    }

    const openEditProvider = (item) => {
        setEditingProvider(item)
        setProviderForm(providerToForm(item))
        setProviderModalOpen(true)
    }

    const closeProviderModal = () => {
        setProviderModalOpen(false)
        setEditingProvider(null)
        setProviderForm(providerToForm(null))
    }

    const setProviderFieldLocal = (field, value) => {
        setProviderForm(prev => ({ ...prev, [field]: value }))
    }

    const saveProviders = async () => {
        const current = Array.isArray(providersConfig.providers) ? providersConfig.providers : []
        const nextProviders = current.filter(item => item.id !== providerForm.id)
        const nextItem = formToProvider(providerForm, editingProvider || {})
        nextProviders.push(nextItem)
        const payload = {
            external_ai_providers: {
                active: providerForm.id || editingProvider?.id || nextItem.id,
                providers: nextProviders,
            },
        }
        await apiFetch('/admin/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        })
        onRefresh()
        closeProviderModal()
    }

    const deleteProvider = async (id) => {
        if (!confirm(t('providerManager.deleteConfirm'))) return
        const current = Array.isArray(providersConfig.providers) ? providersConfig.providers : []
        const nextProviders = current.filter(item => item.id !== id)
        const active = providersConfig.active === id ? (nextProviders[0]?.id || '') : providersConfig.active
        await apiFetch('/admin/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                external_ai_providers: {
                    active,
                    providers: nextProviders,
                },
            }),
        })
        onRefresh()
    }

    const setActiveProvider = async (id) => {
        await apiFetch('/admin/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                external_ai_providers: {
                    active: id,
                    providers: providersConfig.providers,
                },
            }),
        })
        onRefresh()
    }

    return (
        <div className="space-y-6">
            {Boolean(config?.env_source_present) && (
                <div className={`rounded-xl border px-4 py-3 text-sm ${
                    config?.env_writeback_enabled
                        ? (config?.env_backed ? 'border-amber-500/30 bg-amber-500/10 text-amber-600' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600')
                        : 'border-amber-500/30 bg-amber-500/10 text-amber-600'
                }`}>
                    <p className="font-medium">
                        {config?.env_writeback_enabled
                            ? (config?.env_backed
                                ? t('accountManager.envModeWritebackPendingTitle')
                                : t('accountManager.envModeWritebackActiveTitle'))
                            : t('accountManager.envModeRiskTitle')}
                    </p>
                    <p className="mt-1 text-xs opacity-90">
                        {config?.env_writeback_enabled
                            ? t('accountManager.envModeWritebackDesc', { path: config?.config_path || 'config.json' })
                            : t('accountManager.envModeRiskDesc')}
                    </p>
                </div>
            )}

            <ApiKeysPanel
                t={t}
                config={config}
                onAddKey={openAddKey}
                onEditKey={openEditKey}
                copiedKey={copiedKey}
                setCopiedKey={setCopiedKey}
                onDeleteKey={deleteKey}
            />

            <AccountsTable
                t={t}
                providersConfig={providersConfig}
                onAddProvider={openAddProvider}
                onEditProvider={openEditProvider}
                onDeleteProvider={deleteProvider}
                onSetActiveProvider={setActiveProvider}
                onRefreshProvider={refreshProviderFromConfig}
                refreshingProvider={refreshingProvider}
                savingProvider={savingProvider}
            />

            <AddKeyModal
                show={showAddKey}
                t={t}
                editingKey={editingKey}
                newKey={newKey}
                setNewKey={setNewKey}
                loading={loading}
                onClose={closeKeyModal}
                onAdd={addKey}
            />

            {providerModalOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
                    <div className="bg-card w-full max-w-lg rounded-xl border border-border shadow-2xl overflow-hidden">
                        <div className="p-4 border-b border-border flex justify-between items-center">
                            <h3 className="font-semibold">{editingProvider ? t('providerManager.editProvider') : t('providerManager.addProvider')}</h3>
                            <button onClick={closeProviderModal} className="text-muted-foreground hover:text-foreground">×</button>
                        </div>
                        <div className="p-6 space-y-4">
                            <div>
                                <label className="block text-sm font-medium mb-1.5">{t('providerManager.nameLabel')}</label>
                                <input className="input-field" value={providerForm.name} onChange={e => setProviderFieldLocal('name', e.target.value)} />
                            </div>
                            <div>
                                <label className="block text-sm font-medium mb-1.5">{t('providerManager.urlLabel')}</label>
                                <input className="input-field" value={providerForm.base_url} onChange={e => setProviderFieldLocal('base_url', e.target.value)} />
                            </div>
                            <div>
                                <label className="block text-sm font-medium mb-1.5">{t('providerManager.apiKeyLabel')}</label>
                                <input className="input-field" type="password" value={providerForm.api_key} onChange={e => setProviderFieldLocal('api_key', e.target.value)} />
                            </div>
                            <div>
                                <label className="block text-sm font-medium mb-1.5">{t('providerManager.modelLabel')}</label>
                                <input className="input-field" value={providerForm.model} onChange={e => setProviderFieldLocal('model', e.target.value)} />
                            </div>
                            <div>
                                <label className="block text-sm font-medium mb-1.5">{t('providerManager.modeLabel')}</label>
                                <select className="input-field" value={providerForm.mode} onChange={e => setProviderFieldLocal('mode', e.target.value)}>
                                    <option value="auto">auto</option>
                                    <option value="openai">openai</option>
                                    <option value="claude">claude</option>
                                    <option value="gemini">gemini</option>
                                </select>
                            </div>
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">{t('providerManager.maxInflightLabel')}</label>
                                    <input className="input-field" type="number" min="0" value={providerForm.max_inflight} onChange={e => setProviderFieldLocal('max_inflight', e.target.value)} />
                                    <p className="text-xs text-muted-foreground mt-1.5">{t('providerManager.maxInflightHelp')}</p>
                                </div>
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">{t('providerManager.maxQueueLabel')}</label>
                                    <input className="input-field" type="number" min="0" value={providerForm.max_queue} onChange={e => setProviderFieldLocal('max_queue', e.target.value)} />
                                    <p className="text-xs text-muted-foreground mt-1.5">{t('providerManager.maxQueueHelp')}</p>
                                </div>
                            </div>
                            <div className="flex justify-end gap-2 pt-2">
                                <button onClick={closeProviderModal} className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium">{t('actions.cancel')}</button>
                                <button onClick={saveProviders} disabled={savingProvider} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50">
                                    {savingProvider ? t('providerManager.saving') : t('providerManager.save')}
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}
