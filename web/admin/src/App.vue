<template>
  <div class="container-fluid p-0">
    <div class="row g-0">
      <!-- Sidebar -->
      <div class="col-md-3 col-lg-2 d-md-block bg-white sidebar py-4">
        <div class="px-3 mb-4 d-flex align-items-center">
          <i class="bi bi-robot fs-3 text-primary me-2"></i>
          <span class="fs-5 fw-bold text-dark">Catgirl Runtime</span>
        </div>
        <ul class="nav flex-column px-2">
          <li class="nav-item">
            <a class="nav-link" :class="{ 'active': activeTab === 'dashboard' }" @click.prevent="activeTab = 'dashboard'" href="#">
              <i class="bi bi-speedometer2 me-2"></i> Dashboard
            </a>
          </li>
          <li class="nav-item">
            <a class="nav-link" :class="{ 'active': activeTab === 'llm' }" @click.prevent="activeTab = 'llm'" href="#">
              <i class="bi bi-cpu me-2"></i> AI Models & Prompts
            </a>
          </li>
          <li class="nav-item">
            <a class="nav-link" :class="{ 'active': activeTab === 'telegram' }" @click.prevent="activeTab = 'telegram'" href="#">
              <i class="bi bi-telegram me-2"></i> Telegram Integration
            </a>
          </li>
          <li class="nav-item">
            <a class="nav-link" :class="{ 'active': activeTab === 'pool' }" @click.prevent="activeTab = 'pool'" href="#">
              <i class="bi bi-people me-2"></i> Agent Resources
            </a>
          </li>
        </ul>
      </div>

      <!-- Main Content -->
      <main class="col-md-9 ms-sm-auto col-lg-10 px-md-5 py-4">
        <div class="d-flex justify-content-between flex-wrap flex-md-nowrap align-items-center pb-2 mb-4 border-bottom">
          <h1 class="h2 fw-bold text-dark">
            {{ activeTab === 'dashboard' ? 'Dashboard' :
               activeTab === 'llm' ? 'AI Models & Prompts' :
               activeTab === 'telegram' ? 'Telegram Integration' : 'Agent Resources' }}
          </h1>
          <div class="btn-toolbar mb-2 mb-md-0" v-if="activeTab !== 'dashboard'">
            <button @click="saveConfig" type="button" class="btn btn-primary shadow-sm" :disabled="saving || !config">
              <span v-if="saving" class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
              <i v-else class="bi bi-save me-1"></i>
              {{ saving ? 'Saving Changes...' : 'Save Configuration' }}
            </button>
          </div>
        </div>

        <!-- Loading State -->
        <div v-if="!config && activeTab !== 'dashboard'" class="text-center p-5">
          <div class="spinner-border text-primary" role="status" style="width: 3rem; height: 3rem;">
            <span class="visually-hidden">Loading...</span>
          </div>
          <p class="mt-3 text-muted">Fetching runtime configuration...</p>
        </div>

        <div v-if="config || activeTab === 'dashboard'">

          <!-- DASHBOARD TAB -->
          <div v-if="activeTab === 'dashboard'">
            <div class="row">
              <div class="col-md-6 col-lg-4 mb-4">
                <div class="card h-100">
                  <div class="card-body">
                    <div class="d-flex justify-content-between align-items-start">
                      <div>
                        <h6 class="text-muted fw-semibold mb-1">API Server Status</h6>
                        <h3 class="fw-bold mb-0 text-success">
                          <i class="bi bi-check-circle-fill me-1"></i> Online
                        </h3>
                      </div>
                      <div class="p-2 bg-success bg-opacity-10 rounded">
                        <i class="bi bi-hdd-network text-success fs-4"></i>
                      </div>
                    </div>
                    <div class="mt-3 small text-muted" v-if="health">
                      Port: <strong>{{ health.config?.server_port }}</strong>
                    </div>
                  </div>
                </div>
              </div>

              <div class="col-md-6 col-lg-4 mb-4">
                <div class="card h-100">
                  <div class="card-body">
                    <div class="d-flex justify-content-between align-items-start">
                      <div>
                        <h6 class="text-muted fw-semibold mb-1">Database Connection</h6>
                        <h3 class="fw-bold mb-0" :class="health?.database?.healthy ? 'text-success' : 'text-danger'">
                          <i class="bi bi-database me-1"></i> {{ health?.database?.healthy ? 'Healthy' : 'Error' }}
                        </h3>
                      </div>
                      <div class="p-2 bg-primary bg-opacity-10 rounded">
                        <i class="bi bi-database-check text-primary fs-4"></i>
                      </div>
                    </div>
                    <div class="mt-3 small text-muted" v-if="health">
                      Latency: <strong>{{ health.database?.latency_ms }}ms</strong>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="card">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>System Metrics Overview</span>
                <button class="btn btn-sm btn-outline-secondary" @click="fetchHealth">
                  <i class="bi bi-arrow-clockwise"></i> Refresh
                </button>
              </div>
              <div class="card-body bg-light">
                <pre class="mb-0 text-dark" style="font-family: 'Courier New', Courier, monospace; font-size: 0.85rem;">{{ JSON.stringify(health, null, 2) }}</pre>
              </div>
            </div>
          </div>

          <!-- LLM CONFIG TAB -->
          <div v-if="activeTab === 'llm'">
            <div class="card mb-4">
              <div class="card-header">Default Prompts & Tool Access</div>
              <div class="card-body">
                <div class="row">
                  <div class="col-md-6">
                    <label class="form-label fw-semibold">Orchestrator System Prompt</label>
                    <p class="text-muted small mb-2">Used by the main Catgirl loop. Defines personality and directs her to use tools.</p>
                    <textarea class="form-control font-monospace text-muted mb-3" v-model="config.llm.default_system_prompt" rows="3"></textarea>

                    <label class="form-label fw-semibold">Orchestrator Tools (comma separated)</label>
                    <p class="text-muted small mb-2">Which tools the main orchestrator loop is allowed to use.</p>
                    <input type="text" class="form-control form-control-sm mb-4" :value="config.llm.default_orchestrator_tools?.join(', ')" @input="e => updateTools(config.llm, 'default_orchestrator_tools', (e.target as HTMLInputElement).value)">
                  </div>

                  <div class="col-md-6">
                    <label class="form-label fw-semibold">Worker Agent Prompt</label>
                    <p class="text-muted small mb-2">Defines behavior of sub-agents. Instructs them to use SET_STATE. %s is replaced by task.</p>
                    <textarea class="form-control font-monospace text-muted mb-3" v-model="config.llm.default_agent_system_prompt" rows="3"></textarea>

                    <label class="form-label fw-semibold">Agent Tools (comma separated)</label>
                    <p class="text-muted small mb-2">Which tools the sub-agents are allowed to use to perform work.</p>
                    <input type="text" class="form-control form-control-sm" :value="config.llm.default_agent_tools?.join(', ')" @input="e => updateTools(config.llm, 'default_agent_tools', (e.target as HTMLInputElement).value)">
                  </div>
                </div>
              </div>
            </div>

            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>General Purpose LLM Providers</span>
                <button class="btn btn-sm btn-primary" @click="addProvider('providers')">
                  <i class="bi bi-plus"></i> Add Provider
                </button>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th class="ps-4">Base URL</th>
                        <th>API Key</th>
                        <th>Models</th>
                        <th class="text-end pe-4">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!config.llm.providers || config.llm.providers.length === 0">
                        <td colspan="4" class="text-center py-4 text-muted">No providers configured. Click "Add Provider" above.</td>
                      </tr>
                      <tr v-for="(provider, index) in config.llm.providers" :key="'gp'+index">
                        <td class="ps-4">
                          <input type="text" class="form-control form-control-sm" v-model="provider.base_url" placeholder="https://api.openai.com/v1">
                        </td>
                        <td>
                          <div class="input-group input-group-sm">
                            <span class="input-group-text"><i class="bi bi-key"></i></span>
                            <input type="password" class="form-control" v-model="provider.api_key" placeholder="sk-...">
                          </div>
                        </td>
                        <td>
                          <input type="text" class="form-control form-control-sm" :value="provider.models ? provider.models.join(', ') : ''" @input="e => updateModels(provider, (e.target as HTMLInputElement).value)" placeholder="gpt-4o, claude-3">
                        </td>
                        <td class="text-end pe-4">
                          <button class="btn btn-sm btn-outline-danger" @click="config.llm.providers.splice(index, 1)" title="Remove Provider">
                            <i class="bi bi-trash"></i>
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Reasoner Providers -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Reasoner LLM Providers</span>
                <button class="btn btn-sm btn-primary" @click="addProvider('reasoner_providers')">
                  <i class="bi bi-plus"></i> Add Provider
                </button>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th class="ps-4">Base URL</th>
                        <th>API Key</th>
                        <th>Models</th>
                        <th class="text-end pe-4">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!config.llm.reasoner_providers || config.llm.reasoner_providers.length === 0">
                        <td colspan="4" class="text-center py-4 text-muted">No reasoner providers configured.</td>
                      </tr>
                      <tr v-for="(provider, index) in config.llm.reasoner_providers" :key="'reasoner'+index">
                        <td class="ps-4">
                          <input type="text" class="form-control form-control-sm" v-model="provider.base_url" placeholder="https://api.openai.com/v1">
                        </td>
                        <td>
                          <div class="input-group input-group-sm">
                            <span class="input-group-text"><i class="bi bi-key"></i></span>
                            <input type="password" class="form-control" v-model="provider.api_key" placeholder="sk-...">
                          </div>
                        </td>
                        <td>
                          <input type="text" class="form-control form-control-sm" :value="provider.models ? provider.models.join(', ') : ''" @input="e => updateModels(provider, (e.target as HTMLInputElement).value)" placeholder="gpt-4o, claude-3">
                        </td>
                        <td class="text-end pe-4">
                          <button class="btn btn-sm btn-outline-danger" @click="config.llm.reasoner_providers.splice(index, 1)" title="Remove Provider">
                            <i class="bi bi-trash"></i>
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Embedding Providers -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Embedding LLM Providers</span>
                <button class="btn btn-sm btn-primary" @click="addProvider('embedding_providers')">
                  <i class="bi bi-plus"></i> Add Provider
                </button>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th class="ps-4">Base URL</th>
                        <th>API Key</th>
                        <th>Models</th>
                        <th class="text-end pe-4">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!config.llm.embedding_providers || config.llm.embedding_providers.length === 0">
                        <td colspan="4" class="text-center py-4 text-muted">No embedding providers configured.</td>
                      </tr>
                      <tr v-for="(provider, index) in config.llm.embedding_providers" :key="'embedding'+index">
                        <td class="ps-4">
                          <input type="text" class="form-control form-control-sm" v-model="provider.base_url" placeholder="https://api.openai.com/v1">
                        </td>
                        <td>
                          <div class="input-group input-group-sm">
                            <span class="input-group-text"><i class="bi bi-key"></i></span>
                            <input type="password" class="form-control" v-model="provider.api_key" placeholder="sk-...">
                          </div>
                        </td>
                        <td>
                          <input type="text" class="form-control form-control-sm" :value="provider.models ? provider.models.join(', ') : ''" @input="e => updateModels(provider, (e.target as HTMLInputElement).value)" placeholder="text-embedding-3-large">
                        </td>
                        <td class="text-end pe-4">
                          <button class="btn btn-sm btn-outline-danger" @click="config.llm.embedding_providers.splice(index, 1)" title="Remove Provider">
                            <i class="bi bi-trash"></i>
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <div class="row">
              <div class="col-md-6">
                <div class="card">
                  <div class="card-header">Token Limits</div>
                  <div class="card-body">
                    <div class="mb-3">
                      <label class="form-label fw-semibold">Max Completion Tokens</label>
                      <input type="number" class="form-control" v-model="config.llm.max_tokens">
                    </div>
                    <div class="mb-0">
                      <label class="form-label fw-semibold">API Timeout (Seconds)</label>
                      <input type="number" class="form-control" v-model="config.llm.timeout_seconds">
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- TELEGRAM TAB -->
          <div v-if="activeTab === 'telegram'">
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Telegram Bots & Session Overrides</span>
                <button class="btn btn-sm btn-primary" @click="addBot()">
                  <i class="bi bi-plus"></i> Add Telegram Bot
                </button>
              </div>
              <div class="card-body">
                <div v-if="!config.telegram.bots || config.telegram.bots.length === 0" class="text-center py-4 text-muted">
                  No telegram bots configured. Click "Add Telegram Bot" above.
                </div>

                <div v-for="(bot, index) in config.telegram.bots" :key="'bot'+index" class="card mb-4 border-primary">
                  <div class="card-header bg-primary text-white d-flex justify-content-between py-2 align-items-center">
                    <span class="fw-bold">Bot #{{ index + 1 }}</span>
                    <button class="btn btn-sm btn-outline-light" @click="config.telegram.bots.splice(index, 1)" title="Remove Bot">
                      <i class="bi bi-trash"></i>
                    </button>
                  </div>
                  <div class="card-body">
                    <div class="row">
                      <div class="col-md-6 mb-3">
                        <label class="form-label fw-semibold">Bot API Token</label>
                        <div class="input-group input-group-sm">
                          <span class="input-group-text"><i class="bi bi-robot"></i></span>
                          <input type="password" class="form-control" v-model="bot.bot_token" placeholder="1234567890:AAH...">
                        </div>
                      </div>
                      <div class="col-md-6 mb-3">
                        <label class="form-label fw-semibold">Public Webhook URL</label>
                        <div class="input-group input-group-sm">
                          <span class="input-group-text"><i class="bi bi-globe"></i></span>
                          <input type="url" class="form-control" v-model="bot.webhook_url" placeholder="https://your-domain.com/telegram/webhook">
                        </div>
                      </div>
                    </div>

                    <div class="accordion" :id="'botAccordion'+index">
                      <div class="accordion-item">
                        <h2 class="accordion-header">
                          <button class="accordion-button collapsed py-2" type="button" data-bs-toggle="collapse" :data-bs-target="'#collapse'+index">
                            Session Overrides (Prompts, Tools, Models)
                          </button>
                        </h2>
                        <div :id="'collapse'+index" class="accordion-collapse collapse" :data-bs-parent="'#botAccordion'+index">
                          <div class="accordion-body bg-light">

                            <div class="row mb-3">
                              <div class="col-md-6">
                                <label class="form-label fw-semibold small mb-1">Orchestrator System Prompt (Override)</label>
                                <textarea class="form-control font-monospace small text-muted mb-2" v-model="bot.orchestrator_system_prompt" rows="2" placeholder="Leave blank to use default"></textarea>

                                <label class="form-label fw-semibold small mb-1">Allowed Orchestrator Tools</label>
                                <input type="text" class="form-control form-control-sm" :value="bot.allowed_orchestrator_tools?.join(', ')" @input="e => updateTools(bot, 'allowed_orchestrator_tools', (e.target as HTMLInputElement).value)" placeholder="Leave blank to use default">
                              </div>

                              <div class="col-md-6">
                                <label class="form-label fw-semibold small mb-1">Agent System Prompt (Override)</label>
                                <textarea class="form-control font-monospace small text-muted mb-2" v-model="bot.agent_system_prompt" rows="2" placeholder="Leave blank to use default"></textarea>

                                <label class="form-label fw-semibold small mb-1">Allowed Agent Tools</label>
                                <input type="text" class="form-control form-control-sm" :value="bot.allowed_agent_tools?.join(', ')" @input="e => updateTools(bot, 'allowed_agent_tools', (e.target as HTMLInputElement).value)" placeholder="Leave blank to use default">
                              </div>
                            </div>

                            <div class="row">
                              <div class="col-md-6">
                                <label class="form-label fw-semibold small mb-1">Pin GP Model</label>
                                <input type="text" class="form-control form-control-sm" v-model="bot.gp_model" placeholder="eg. claude-3-opus-20240229">
                                <div class="form-text" style="font-size: 0.7rem">Forces all tasks in this session to use this specific model.</div>
                              </div>
                              <div class="col-md-6">
                                <label class="form-label fw-semibold small mb-1">Pin Reasoner Model</label>
                                <input type="text" class="form-control form-control-sm" v-model="bot.reasoner_model" placeholder="eg. gpt-4-turbo">
                              </div>
                            </div>

                          </div>
                        </div>
                      </div>
                    </div>

                  </div>
                </div>
              </div>
            </div>

            <div class="card mb-4">
              <div class="card-header">Server Bind Settings</div>
              <div class="card-body">
                <div class="mb-2">
                  <label class="form-label fw-semibold">Local Listen Address</label>
                  <input type="text" class="form-control" v-model="config.telegram.listen_addr" placeholder="0.0.0.0:8081">
                  <div class="form-text">The local interface and port that the Golang application will bind to for receiving ALL webhook POST requests.</div>
                </div>
              </div>
            </div>

          </div>

          <!-- POOL TAB -->
          <div v-if="activeTab === 'pool'">
            <div class="row">
              <div class="col-lg-6">
                <div class="card">
                  <div class="card-header">Agent Concurrency</div>
                  <div class="card-body">
                    <p class="text-muted small mb-4">Control how many simultaneous LLM agent loops can run at once. Higher numbers consume more memory and API rate limits.</p>

                    <div class="row">
                      <div class="col-6 mb-3">
                        <label class="form-label fw-semibold">Minimum Warm Agents</label>
                        <input type="number" class="form-control" v-model="config.agent_pool.min_agents">
                      </div>
                      <div class="col-6 mb-3">
                        <label class="form-label fw-semibold">Maximum Active Agents</label>
                        <input type="number" class="form-control" v-model="config.agent_pool.max_agents">
                      </div>
                      <div class="col-12">
                        <label class="form-label fw-semibold">Agent Idle Timeout (Seconds)</label>
                        <input type="number" class="form-control" v-model="config.agent_pool.idle_timeout_seconds">
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <div class="col-lg-6">
                <div class="card">
                  <div class="card-header">Task Queue Limits</div>
                  <div class="card-body">
                    <p class="text-muted small mb-4">Set boundaries for task execution trees to prevent infinite recursive loops from LLM hallucinations.</p>

                    <div class="mb-3">
                      <label class="form-label fw-semibold">Max Task Recursion Depth</label>
                      <input type="number" class="form-control" v-model="config.global.max_task_depth">
                      <div class="form-text">How many layers deep a task can spawn sub-tasks.</div>
                    </div>
                    <div>
                      <label class="form-label fw-semibold">Global Queue Size</label>
                      <input type="number" class="form-control" v-model="config.global.max_queue_size">
                      <div class="form-text">Maximum number of pending tasks before rejecting new spawns.</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

        </div>
      </main>
    </div>

    <!-- Toast Notification Container -->
    <div class="toast-container position-fixed bottom-0 end-0 p-3">
      <div id="saveToast" class="toast align-items-center text-bg-success border-0" role="alert" aria-live="assertive" aria-atomic="true">
        <div class="d-flex">
          <div class="toast-body">
            <i class="bi bi-check-circle me-2"></i> Configuration saved successfully!
          </div>
          <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast" aria-label="Close"></button>
        </div>
      </div>

      <div id="errorToast" class="toast align-items-center text-bg-danger border-0" role="alert" aria-live="assertive" aria-atomic="true">
        <div class="d-flex">
          <div class="toast-body">
            <i class="bi bi-exclamation-triangle me-2"></i> <span id="errorToastMessage">Failed to save configuration.</span>
          </div>
          <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast" aria-label="Close"></button>
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
const activeTab = ref('dashboard')

// @ts-ignore - Bootstrap is loaded globally via CDN
const showToast = (id: string) => {
  const toastEl = document.getElementById(id)
  if (toastEl) {
    // @ts-ignore
    const toast = new bootstrap.Toast(toastEl)
    toast.show()
  }
}

const fetchConfig = async () => {
  try {
    const res = await fetch('/api/v1/config')
    const data = await res.json()
    config.value = data.config

    // Ensure nested arrays exist so UI doesn't crash
    if (!config.value.llm.providers) config.value.llm.providers = []
    if (!config.value.llm.reasoner_providers) config.value.llm.reasoner_providers = []
    if (!config.value.llm.embedding_providers) config.value.llm.embedding_providers = []

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
      document.getElementById('errorToastMessage')!.innerText = err.error || 'Failed to save configuration.'
      showToast('errorToast')
    } else {
      showToast('saveToast')
    }
  } catch (err) {
    console.error(err)
    document.getElementById('errorToastMessage')!.innerText = 'Network error while saving.'
    showToast('errorToast')
  } finally {
    saving.value = false
  }
}

const updateTools = (target: any, propKey: string, val: string) => {
  target[propKey] = val.split(',').map(s => s.trim()).filter(s => s.length > 0)
}

const addBot = () => {
  if (!config.value.telegram.bots) config.value.telegram.bots = []
  config.value.telegram.bots.push({
    bot_token: '',
    webhook_url: '',
    orchestrator_system_prompt: '',
    agent_system_prompt: '',
    allowed_orchestrator_tools: [],
    allowed_agent_tools: [],
    gp_model: '',
    reasoner_model: ''
  })
}
  provider.models = val.split(',').map(s => s.trim()).filter(s => s.length > 0)
}

const updateModels = (provider: any, val: string) => {
  if (!config.value.llm[listName]) {
    config.value.llm[listName] = []
  }
  config.value.llm[listName].push({
    base_url: 'https://api.openai.com/v1',
    api_key: '',
    models: ['gpt-4o']
  })
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
