import exampleConfig from '../../../config.example.json'

export function getBatchImportTemplates(t) {
    return {
        full: {
            name: t('batchImport.templates.full.name'),
            desc: t('batchImport.templates.full.desc'),
            config: {
                ...exampleConfig,
                external_ai_providers: {
                    active: 'openai-main',
                    providers: [
                        {
                            id: 'openai-main',
                            name: 'OpenAI Main',
                            base_url: 'https://api.openai.com/v1',
                            api_key: 'sk-your-upstream-key',
                            model: 'gpt-4o-mini',
                            mode: 'openai',
                            max_inflight: 2,
                            max_queue: 0,
                            headers: {},
                        },
                        {
                            id: 'claude-main',
                            name: 'Claude Main',
                            base_url: 'https://api.anthropic.com',
                            api_key: 'sk-ant-your-upstream-key',
                            model: 'claude-sonnet-4-6',
                            mode: 'claude',
                            max_inflight: 2,
                            max_queue: 0,
                            headers: {},
                        },
                    ],
                },
            },
        },
        provider_openai: {
            name: t('batchImport.templates.providerOpenAI.name'),
            desc: t('batchImport.templates.providerOpenAI.desc'),
            config: {
                keys: ['your-gateway-api-key'],
                external_ai: {
                    base_url: 'https://api.openai.com/v1',
                    api_key: 'sk-your-upstream-key',
                    model: 'gpt-4o-mini',
                    mode: 'auto',
                    max_inflight: 2,
                    max_queue: 0,
                    headers: {},
                },
            },
        },
        provider_claude: {
            name: t('batchImport.templates.providerClaude.name'),
            desc: t('batchImport.templates.providerClaude.desc'),
            config: {
                keys: ['your-gateway-api-key'],
                external_ai: {
                    base_url: 'https://api.anthropic.com',
                    api_key: 'sk-ant-your-upstream-key',
                    model: 'claude-sonnet-4-6',
                    mode: 'auto',
                    max_inflight: 2,
                    max_queue: 0,
                    headers: {},
                },
            },
        },
        keys_only: {
            name: t('batchImport.templates.keysOnly.name'),
            desc: t('batchImport.templates.keysOnly.desc'),
            config: {
                keys: ['key-1', 'key-2', 'key-3'],
            },
        },
    }
}
