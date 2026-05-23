import { useCallback, useEffect, useMemo, useState } from 'react'
import { detectRuntimeEnv } from '../utils/runtimeEnv'

export function useAdminAuth({ isProduction, location, t }) {
    const [message, setMessage] = useState(null)
    const [token, setToken] = useState(null)
    const [authChecking, setAuthChecking] = useState(true)

    const isAdminRoute = location.pathname.startsWith('/admin') || isProduction
    const runtimeEnv = useMemo(() => detectRuntimeEnv(), [])
    const isVercel = runtimeEnv.isVercel

    const showMessage = useCallback((type, text) => {
        setMessage({ type, text })
        setTimeout(() => setMessage(null), 5000)
    }, [])

    const handleLogout = useCallback(() => {
        setToken(null)
        localStorage.removeItem('tool-gateway_token')
        localStorage.removeItem('tool-gateway_token_expires')
        sessionStorage.removeItem('tool-gateway_token')
        sessionStorage.removeItem('tool-gateway_token_expires')
    }, [])

    const handleLogin = useCallback((newToken) => {
        setToken(newToken)
    }, [])

    useEffect(() => {
        if (!isAdminRoute) {
            setAuthChecking(false)
            return
        }

        const checkAuth = async () => {
            const storedToken = localStorage.getItem('tool-gateway_token') || sessionStorage.getItem('tool-gateway_token')
            const expiresAt = parseInt(localStorage.getItem('tool-gateway_token_expires') || sessionStorage.getItem('tool-gateway_token_expires') || '0')

            if (storedToken && expiresAt > Date.now()) {
                try {
                    const res = await fetch('/admin/verify', {
                        headers: { 'Authorization': `Bearer ${storedToken}` }
                    })
                    if (res.ok) {
                        setToken(storedToken)
                    } else {
                        handleLogout()
                    }
                } catch {
                    setToken(storedToken)
                }
            }
            setAuthChecking(false)
        }

        checkAuth()
    }, [handleLogout, isAdminRoute, t])

    return {
        token,
        authChecking,
        message,
        isAdminRoute,
        isVercel,
        showMessage,
        handleLogin,
        handleLogout,
    }
}
