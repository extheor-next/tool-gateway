import { useState } from 'react'

import { detectProviderMode, normalizeProviderMode } from './providerMode'

function providerFromConfig(config) {
    const external = config?.external_ai || {}
    const baseURL = external.base_url || external.url || ''
    const apiKey = external.api_key || ''
    const configuredMode = normalizeProviderMode(external.mode)
    const detectedMode = detectProviderMode(baseURL, apiKey)
    return {
        base_url: baseURL,
        api_key: apiKey,
        model: external.model || '',
        mode: configuredMode === 'auto' && detectedMode !== 'auto' ? detectedMode : configuredMode,
    }
}

export function useAccountActions({ apiFetch, t, onMessage, onRefresh, config }) {
    const [showAddKey, setShowAddKey] = useState(false)
    const [editingKey, setEditingKey] = useState(null)
    const [newKey, setNewKey] = useState({ key: '', name: '', remark: '' })
    const [copiedKey, setCopiedKey] = useState(null)
    const [provider, setProvider] = useState(providerFromConfig(config))
    const [savingProvider, setSavingProvider] = useState(false)
    const [refreshingProvider, setRefreshingProvider] = useState(false)
    const [loading, setLoading] = useState(false)

    const openAddKey = () => {
        setEditingKey(null)
        setNewKey({ key: '', name: '', remark: '' })
        setShowAddKey(true)
    }

    const openEditKey = (item) => {
        if (!item?.key) return
        setEditingKey(item)
        setNewKey({
            key: item.key || '',
            name: item.name || '',
            remark: item.remark || '',
        })
        setShowAddKey(true)
    }

    const closeKeyModal = () => {
        setShowAddKey(false)
        setEditingKey(null)
        setNewKey({ key: '', name: '', remark: '' })
    }

    const setProviderField = (field, value) => {
        setProvider(prev => {
            const next = { ...prev, [field]: value }
            if (field === 'base_url' || field === 'api_key') {
                const detected = detectProviderMode(next.base_url, next.api_key)
                if (detected !== 'auto') {
                    next.mode = detected
                }
            }
            if (field === 'mode') {
                next.mode = normalizeProviderMode(value)
            }
            return next
        })
    }

    const refreshProviderFromConfig = async () => {
        setRefreshingProvider(true)
        try {
            await onRefresh()
        } finally {
            setRefreshingProvider(false)
        }
    }

    const saveProvider = async () => {
        const baseURL = provider.base_url.trim()
        const apiKey = provider.api_key.trim()
        if (!baseURL || !apiKey) {
            onMessage('error', t('providerManager.requiredFields'))
            return
        }
        setSavingProvider(true)
        try {
            const res = await apiFetch('/admin/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    external_ai: {
                        base_url: baseURL,
                        api_key: apiKey,
                        model: provider.model.trim(),
                        mode: normalizeProviderMode(provider.mode),
                    },
                }),
            })
            const data = await res.json().catch(() => ({}))
            if (res.ok) {
                onMessage('success', t('providerManager.saveSuccess'))
                onRefresh()
            } else {
                onMessage('error', data.detail || t('messages.requestFailed'))
            }
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setSavingProvider(false)
        }
    }

    const addKey = async () => {
        const isEditing = Boolean(editingKey?.key)
        if (!isEditing && !newKey.key.trim()) {
            return
        }
        setLoading(true)
        try {
            const endpoint = isEditing
                ? `/admin/keys/${encodeURIComponent(editingKey.key)}`
                : '/admin/keys'
            const method = isEditing ? 'PUT' : 'POST'
            const payload = isEditing
                ? { name: newKey.name, remark: newKey.remark }
                : { key: newKey.key.trim(), name: newKey.name, remark: newKey.remark }
            if (!isEditing && !payload.key) {
                return
            }
            const res = await apiFetch(endpoint, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            })
            if (res.ok) {
                onMessage('success', isEditing ? t('accountManager.updateKeySuccess') : t('accountManager.addKeySuccess'))
                closeKeyModal()
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || (isEditing ? t('messages.requestFailed') : t('messages.failedToAdd')))
            }
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }

    const deleteKey = async (key) => {
        if (!confirm(t('accountManager.deleteKeyConfirm'))) return
        try {
            const res = await apiFetch(`/admin/keys/${encodeURIComponent(key)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', t('messages.deleted'))
                onRefresh()
            } else {
                onMessage('error', t('messages.deleteFailed'))
            }
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        }
    }

    return {
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
    }
}
