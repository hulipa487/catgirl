<template>
  <nav class="navbar navbar-expand-lg navbar-dark bg-dark mb-4">
    <div class="container-fluid">
      <a class="navbar-brand text-white" href="#">Catgirl Runtime Admin</a>
    </div>
  </nav>

  <div class="container">
    <div class="row">
      <div class="col-12">
        <div class="card mb-4">
          <div class="card-header bg-primary text-white d-flex justify-content-between align-items-center">
            <h5 class="mb-0">Runtime Configuration</h5>
            <button @click="saveConfig" class="btn btn-sm btn-light" :disabled="saving">
              <span v-if="saving" class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>
              {{ saving ? 'Saving...' : 'Save Configuration' }}
            </button>
          </div>
          <div class="card-body" v-if="config">
            <ul class="nav nav-tabs mb-3" id="configTabs" role="tablist">
              <li class="nav-item" role="presentation">
                <button class="nav-link active" id="llm-tab" data-bs-toggle="tab" data-bs-target="#llm" type="button" role="tab" aria-controls="llm" aria-selected="true">LLM & Agents</button>
              </li>
              <li class="nav-item" role="presentation">
                <button class="nav-link" id="telegram-tab" data-bs-toggle="tab" data-bs-target="#telegram" type="button" role="tab" aria-controls="telegram" aria-selected="false">Telegram</button>
              </li>
              <li class="nav-item" role="presentation">
                <button class="nav-link" id="pool-tab" data-bs-toggle="tab" data-bs-target="#pool" type="button" role="tab" aria-controls="pool" aria-selected="false">Agent Pool</button>
              </li>
            </ul>

            <div class="tab-content" id="configTabsContent">
              <!-- LLM Configuration Tab -->
              <div class="tab-pane fade show active" id="llm" role="tabpanel" aria-labelledby="llm-tab">
                <h6 class="border-bottom pb-2 mt-2">Prompts & Parameters</h6>
                <div class="row mb-3">
                  <div class="col-md-6">
                    <label class="form-label">System Prompt (Main Orchestrator)</label>
                    <textarea class="form-control" v-model="config.llm.system_prompt" rows="3"></textarea>
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">Agent System Prompt (Workers)</label>
                    <textarea class="form-control" v-model="config.llm.agent_system_prompt" rows="3"></textarea>
                  </div>
                </div>
                <div class="row mb-4">
                  <div class="col-md-6">
                    <label class="form-label">Max Tokens</label>
                    <input type="number" class="form-control" v-model="config.llm.max_tokens">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">Timeout (Seconds)</label>
                    <input type="number" class="form-control" v-model="config.llm.timeout_seconds">
                  </div>
                </div>

                <h6 class="border-bottom pb-2">General Purpose LLM Providers</h6>
                <div class="mb-3">
                  <div v-for="(provider, index) in config.llm.providers" :key="'gp'+index" class="card mb-2 border-info">
                    <div class="card-body py-2">
                      <div class="row">
                        <div class="col-md-4">
                          <label class="form-label mb-0 small">Base URL</label>
                          <input type="text" class="form-control form-control-sm" v-model="provider.base_url">
                        </div>
                        <div class="col-md-4">
                          <label class="form-label mb-0 small">API Key</label>
                          <input type="password" class="form-control form-control-sm" v-model="provider.api_key">
                        </div>
                        <div class="col-md-3">
                          <label class="form-label mb-0 small">Models (comma separated)</label>
                          <input type="text" class="form-control form-control-sm" :value="provider.models.join(', ')" @input="e => updateModels(provider, (e.target as HTMLInputElement).value)">
                        </div>
                        <div class="col-md-1 d-flex align-items-end">
                          <button class="btn btn-sm btn-danger w-100" @click="config.llm.providers.splice(index, 1)">X</button>
                        </div>
                      </div>
                    </div>
                  </div>
                  <button class="btn btn-sm btn-outline-info" @click="addProvider('providers')">+ Add Provider</button>
                </div>
              </div>

              <!-- Telegram Configuration Tab -->
              <div class="tab-pane fade" id="telegram" role="tabpanel" aria-labelledby="telegram-tab">
                <div class="mb-3 mt-3">
                  <label class="form-label">Bot Token</label>
                  <input type="password" class="form-control" v-model="config.telegram.bot_token">
                </div>
                <div class="mb-3">
                  <label class="form-label">Webhook URL</label>
                  <input type="url" class="form-control" v-model="config.telegram.webhook_url" placeholder="https://your-domain.com/telegram/webhook">
                  <div class="form-text">Must be HTTPS to receive Telegram webhooks.</div>
                </div>
                <div class="mb-3">
                  <label class="form-label">Listen Address</label>
                  <input type="text" class="form-control" v-model="config.telegram.listen_addr" placeholder="0.0.0.0:8081">
                </div>
              </div>

              <!-- Agent Pool Configuration Tab -->
              <div class="tab-pane fade" id="pool" role="tabpanel" aria-labelledby="pool-tab">
                <div class="row mt-3">
                  <div class="col-md-4 mb-3">
                    <label class="form-label">Min Agents</label>
                    <input type="number" class="form-control" v-model="config.agent_pool.min_agents">
                  </div>
                  <div class="col-md-4 mb-3">
                    <label class="form-label">Max Agents</label>
                    <input type="number" class="form-control" v-model="config.agent_pool.max_agents">
                  </div>
                  <div class="col-md-4 mb-3">
                    <label class="form-label">Idle Timeout (Seconds)</label>
                    <input type="number" class="form-control" v-model="config.agent_pool.idle_timeout_seconds">
                  </div>
                </div>
                <div class="row">
                   <div class="col-md-6 mb-3">
                    <label class="form-label">Global Max Task Depth</label>
                    <input type="number" class="form-control" v-model="config.global.max_task_depth">
                  </div>
                  <div class="col-md-6 mb-3">
                    <label class="form-label">Global Max Queue Size</label>
                    <input type="number" class="form-control" v-model="config.global.max_queue_size">
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="card-body text-center p-5">
            <div class="spinner-border text-primary" role="status">
              <span class="visually-hidden">Loading...</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Health Status View -->
    <div class="row mt-2">
      <div class="col-12">
        <div class="card mb-4">
          <div class="card-header bg-success text-white d-flex justify-content-between align-items-center">
            <h5 class="mb-0">System Health</h5>
            <button @click="fetchHealth" class="btn btn-sm btn-light">Refresh</button>
          </div>
          <div class="card-body" v-if="health">
            <div class="row">
              <div class="col-md-4">
                <h6>API Server</h6>
                <p>Status: <span class="badge bg-success">OK</span></p>
                <p>Port: {{ health.config?.server_port }}</p>
              </div>
              <div class="col-md-8">
                 <h6>Database Ping Test</h6>
                 <pre class="bg-dark text-light p-2 rounded small">{{ JSON.stringify(health.database, null, 2) }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const config = ref<any>(null)
const health = ref<any>(null)
const saving = ref(false)

const fetchConfig = async () => {
  try {
    const res = await fetch('/api/v1/config')
    const data = await res.json()
    config.value = data.config
  } catch (err) {
    console.error('Failed to fetch config', err)
  }
}

const fetchHealth = async () => {
  try {
    const res = await fetch('/health/detailed')
    health.value = await res.json()
  } catch (err) {
    console.error('Failed to fetch health', err)
  }
}

const saveConfig = async () => {
  saving.value = true
  try {
    const res = await fetch('/api/v1/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config.value)
    })
    if (!res.ok) {
      const err = await res.json()
      alert('Failed to save: ' + err.error)
    } else {
      alert('Configuration updated successfully. (Note: Some changes may require restarting the Catgirl runtime service to take full effect)')
    }
  } catch (err) {
    console.error(err)
    alert('Failed to save config')
  } finally {
    saving.value = false
  }
}

const updateModels = (provider: any, val: string) => {
  provider.models = val.split(',').map(s => s.trim()).filter(s => s.length > 0)
}

const addProvider = (listName: 'providers' | 'reasoner_providers' | 'embedding_providers') => {
  if (!config.value.llm[listName]) {
    config.value.llm[listName] = []
  }
  config.value.llm[listName].push({
    base_url: 'https://api.openai.com/v1',
    api_key: '',
    models: ['gpt-4o']
  })
}

onMounted(() => {
  fetchConfig()
  fetchHealth()
})
</script>
