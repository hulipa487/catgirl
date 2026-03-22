<template>
  <!-- Loading / Auth Screen -->
  <div v-if="isLoading" class="d-flex justify-content-center align-items-center vh-100 bg-light">
    <div class="text-center">
      <div class="spinner-border text-primary mb-3" role="status" style="width: 3rem; height: 3rem;">
        <span class="visually-hidden">Loading...</span>
      </div>
      <p class="text-muted">Checking authentication...</p>
    </div>
  </div>

  <div v-else-if="!isAuthenticated" class="d-flex justify-content-center align-items-center vh-100 bg-light">
    <div class="text-center">
      <i class="bi bi-lock fs-1 text-muted mb-3"></i>
      <p class="text-muted">Redirecting to login...</p>
      <button @click="redirectToLogin" class="btn btn-primary">Go to Login</button>
    </div>
  </div>

  <div v-else class="container-fluid p-0">
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
              <i class="bi bi-cpu me-2"></i> LLM Settings
            </a>
          </li>
          <li class="nav-item">
            <a class="nav-link" :class="{ 'active': activeTab === 'telegram' }" @click.prevent="activeTab = 'telegram'" href="#">
              <i class="bi bi-telegram me-2"></i> Telegram Bots
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
               activeTab === 'llm' ? 'LLM Settings' :
               activeTab === 'telegram' ? 'Telegram Bots' : 'Agent Resources' }}
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
            <!-- Status Cards Row -->
            <div class="row">
              <div class="col-md-4 mb-4">
                <div class="card h-100">
                  <div class="card-body">
                    <div class="d-flex justify-content-between align-items-start">
                      <div>
                        <h6 class="text-muted fw-semibold mb-1">API Server</h6>
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

              <div class="col-md-4 mb-4">
                <div class="card h-100">
                  <div class="card-body">
                    <div class="d-flex justify-content-between align-items-start">
                      <div>
                        <h6 class="text-muted fw-semibold mb-1">Database</h6>
                        <h3 class="fw-bold mb-0" :class="health?.database?.healthy ? 'text-success' : 'text-danger'">
                          <i class="bi me-1" :class="health?.database?.healthy ? 'bi-check-circle-fill text-success' : 'bi-x-circle-fill text-danger'"></i>
                          {{ health?.database?.healthy ? 'Healthy' : 'Error' }}
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

              <div class="col-md-4 mb-4">
                <div class="card h-100">
                  <div class="card-body">
                    <div class="d-flex justify-content-between align-items-start">
                      <div>
                        <h6 class="text-muted fw-semibold mb-1">Active Workers</h6>
                        <h3 class="fw-bold mb-0 text-primary">
                          <i class="bi bi-robot me-1"></i>
                          {{ health?.agents?.busy || 0 }}
                        </h3>
                      </div>
                      <div class="p-2 bg-primary bg-opacity-10 rounded">
                        <i class="bi bi-people text-primary fs-4"></i>
                      </div>
                    </div>
                    <div class="mt-3 small text-muted" v-if="health">
                      Idle: <strong>{{ health?.agents?.idle || 0 }}</strong> |
                      Total: <strong>{{ health?.agents?.total || 0 }}</strong>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Global Usage Stats -->
            <div class="row mb-4">
              <div class="col-md-3">
                <div class="card bg-primary text-white">
                  <div class="card-body text-center">
                    <h6 class="text-muted mb-1" style="opacity: 0.8;">Input Tokens</h6>
                    <h2 class="mb-0">{{ formatNumber(health?.usage?.global?.total_input_tokens) }}</h2>
                  </div>
                </div>
              </div>
              <div class="col-md-3">
                <div class="card bg-success text-white">
                  <div class="card-body text-center">
                    <h6 class="text-muted mb-1" style="opacity: 0.8;">Output Tokens</h6>
                    <h2 class="mb-0">{{ formatNumber(health?.usage?.global?.total_output_tokens) }}</h2>
                  </div>
                </div>
              </div>
              <div class="col-md-3">
                <div class="card bg-info text-white">
                  <div class="card-body text-center">
                    <h6 class="text-muted mb-1" style="opacity: 0.8;">Total Tokens</h6>
                    <h2 class="mb-0">{{ formatNumber(health?.usage?.global?.total_tokens) }}</h2>
                  </div>
                </div>
              </div>
              <div class="col-md-3">
                <div class="card bg-secondary text-white">
                  <div class="card-body text-center">
                    <h6 class="text-muted mb-1" style="opacity: 0.8;">Requests</h6>
                    <h2 class="mb-0">{{ formatNumber(health?.usage?.global?.record_count) }}</h2>
                  </div>
                </div>
              </div>
            </div>

            <!-- Per Model Usage -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Usage by Model</span>
                <button class="btn btn-sm btn-outline-secondary" @click="fetchHealth">
                  <i class="bi bi-arrow-clockwise"></i> Refresh
                </button>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th>Model</th>
                        <th class="text-end">Input Tokens</th>
                        <th class="text-end">Output Tokens</th>
                        <th class="text-end">Total Tokens</th>
                        <th class="text-end">Requests</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!health?.usage?.by_model?.length">
                        <td colspan="5" class="text-center py-4 text-muted">No usage data available</td>
                      </tr>
                      <tr v-for="(row, index) in health?.usage?.by_model" :key="'model'+index">
                        <td class="ps-4"><span class="font-monospace">{{ row.model || 'unknown' }}</span></td>
                        <td class="text-end">{{ formatNumber(row.input_tokens) }}</td>
                        <td class="text-end">{{ formatNumber(row.output_tokens) }}</td>
                        <td class="text-end"><strong>{{ formatNumber(row.total_tokens) }}</strong></td>
                        <td class="text-end">{{ formatNumber(row.request_count) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Per Bot Usage -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Usage by Telegram Bot</span>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th>Telegram User ID</th>
                        <th class="text-end">Input Tokens</th>
                        <th class="text-end">Output Tokens</th>
                        <th class="text-end">Total Tokens</th>
                        <th class="text-end">Requests</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!health?.usage?.by_bot?.length">
                        <td colspan="5" class="text-center py-4 text-muted">No bot usage data available</td>
                      </tr>
                      <tr v-for="(row, index) in health?.usage?.by_bot" :key="'bot'+index">
                        <td class="ps-4"><strong>{{ row.telegram_user_id }}</strong></td>
                        <td class="text-end">{{ formatNumber(row.input_tokens) }}</td>
                        <td class="text-end">{{ formatNumber(row.output_tokens) }}</td>
                        <td class="text-end"><strong>{{ formatNumber(row.total_tokens) }}</strong></td>
                        <td class="text-end">{{ formatNumber(row.request_count) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <!-- Sessions List -->
            <div class="card mb-4">
              <div class="card-header">
                <span>Active Sessions</span>
              </div>
              <div class="card-body p-0">
                <div class="table-responsive">
                  <table class="table table-hover align-middle mb-0">
                    <thead class="table-light">
                      <tr>
                        <th>Session ID</th>
                        <th>Telegram User</th>
                        <th>Status</th>
                        <th>Created</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-if="!sessions?.length">
                        <td colspan="4" class="text-center py-4 text-muted">No sessions found</td>
                      </tr>
                      <tr v-for="session in sessions" :key="session.id">
                        <td class="ps-4"><span class="font-monospace small">{{ session.id.substring(0, 8) }}...</span></td>
                        <td><strong>{{ session.telegram_user_id }}</strong></td>
                        <td><span class="badge bg-success">{{ session.status }}</span></td>
                        <td class="text-muted small">{{ formatDate(session.created_at) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </div>

          <!-- LLM SETTINGS TAB -->
          <div v-if="activeTab === 'llm'">
            <!-- LLM Providers -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>LLM Providers (Global - used for GP and Reasoner models)</span>
                <button class="btn btn-sm btn-primary" @click="addLLMProvider('providers')">
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
                        <td colspan="4" class="text-center py-4 text-muted">No providers configured.</td>
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
                          <input type="text" class="form-control form-control-sm" :value="provider.models ? provider.models.join(', ') : ''" @input="e => updateModels(provider, (e.target as HTMLInputElement).value)" placeholder="gpt-4o, gpt-4-turbo">
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

            <!-- Embedding Providers -->
            <div class="card mb-4">
              <div class="card-header d-flex justify-content-between align-items-center">
                <span>Embedding LLM Providers (Global)</span>
                <button class="btn btn-sm btn-primary" @click="addLLMProvider('embedding_providers')">
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
                      <tr v-for="(provider, index) in config.llm.embedding_providers" :key="'emb'+index">
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
                <span>Telegram Bots</span>
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
                    <!-- Bot Basic Info -->
                    <div class="row mb-4">
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

                    <!-- Model Selection (pins to global providers) -->
                    <h5 class="border-bottom pb-2 mb-3">Model Selection</h5>
                    <div class="row mb-4">
                      <div class="col-md-6">
                        <label class="form-label fw-semibold">Pin GP Model</label>
                        <input type="text" class="form-control" v-model="bot.gp_model" placeholder="eg. claude-3-opus-20240229">
                        <div class="form-text" style="font-size: 0.7rem">Leave blank to use random model from providers.</div>
                      </div>
                      <div class="col-md-6">
                        <label class="form-label fw-semibold">Pin Reasoner Model</label>
                        <input type="text" class="form-control" v-model="bot.reasoner_model" placeholder="eg. gpt-4-turbo">
                        <div class="form-text" style="font-size: 0.7rem">Leave blank to use random model from reasoner providers.</div>
                      </div>
                    </div>

                    <!-- Prompts & Tools Section -->
                    <h5 class="border-bottom pb-2 mb-3">Prompts & Tools</h5>

                    <div class="row">
                      <div class="col-md-6">
                        <label class="form-label fw-semibold">Orchestrator System Prompt</label>
                        <p class="text-muted small mb-2">Defines personality and directs the agent to use tools.</p>
                        <textarea class="form-control font-monospace text-muted mb-3" v-model="bot.orchestrator_system_prompt" rows="3" placeholder="You are an autonomous agent..."></textarea>

                        <label class="form-label fw-semibold">Allowed Orchestrator Tools</label>
                        <p class="text-muted small mb-2">Tools the main orchestrator loop is allowed to use.</p>
                        <div class="mb-3">
                          <div class="form-check form-switch" v-for="tool in availableTools" :key="'orch_'+tool">
                            <input class="form-check-input" type="checkbox" role="switch" :id="'orch_tool_'+index+'_'+tool" :value="tool" v-model="bot.allowed_orchestrator_tools">
                            <label class="form-check-label font-monospace small" :for="'orch_tool_'+index+'_'+tool">{{ tool }}</label>
                          </div>
                        </div>
                      </div>

                      <div class="col-md-6">
                        <label class="form-label fw-semibold">Worker Agent Prompt</label>
                        <p class="text-muted small mb-2">Defines behavior of sub-agents. %s is replaced by task.</p>
                        <textarea class="form-control font-monospace text-muted mb-3" v-model="bot.agent_system_prompt" rows="3" placeholder="You are a worker agent..."></textarea>

                        <label class="form-label fw-semibold">Allowed Agent Tools</label>
                        <p class="text-muted small mb-2">Tools the sub-agents are allowed to use.</p>
                        <div class="mb-3">
                          <div class="form-check form-switch" v-for="tool in availableTools" :key="'agent_'+tool">
                            <input class="form-check-input" type="checkbox" role="switch" :id="'agent_tool_'+index+'_'+tool" :value="tool" v-model="bot.allowed_agent_tools">
                            <label class="form-check-label font-monospace small" :for="'agent_tool_'+index+'_'+tool">{{ tool }}</label>
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
import { ref, onMounted, onUnmounted } from 'vue'

const config = ref<any>(null)
const health = ref<any>(null)
const sessions = ref<any[]>([])
const saving = ref(false)
const activeTab = ref('dashboard')
const isAuthenticated = ref(false)
const isLoading = ref(true)

const availableTools = ref<string[]>([])

// @ts-ignore - Bootstrap is loaded globally via CDN
const showToast = (id: string) => {
  const toastEl = document.getElementById(id)
  if (toastEl) {
    // @ts-ignore
    const toast = new bootstrap.Toast(toastEl)
    toast.show()
  }
}

let healthInterval: any;

// Check authentication by trying to fetch config
// If 401/403, redirect to mtfpass login
const checkAuth = async (): Promise<boolean> => {
  try {
    const res = await fetch('/api/v1/config', {
      credentials: 'include'
    })
    if (!res.ok) {
      return false
    }
    return true
  } catch {
    return false
  }
}

const redirectToLogin = () => {
  const currentUrl = window.location.origin + window.location.pathname
  window.location.href = `https://auth.mtf.edu.ci/auth/login?origin=${encodeURIComponent(currentUrl)}`
}

// Helper functions
const formatNumber = (num: any): string => {
  if (num === null || num === undefined) return '0'
  return Number(num).toLocaleString()
}

const formatDate = (dateStr: string): string => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleDateString() + ' ' + date.toLocaleTimeString()
}

const fetchSessions = async () => {
  try {
    const res = await fetch('/api/v1/sessions', {
      credentials: 'include'
    })
    if (!res.ok) {
      redirectToLogin()
      return
    }
    const data = await res.json()
    sessions.value = data.sessions || []
  } catch (err) {
    console.error('Failed to fetch sessions', err)
  }
}

const fetchConfig = async () => {
  try {
    const res = await fetch('/api/v1/config', {
      credentials: 'include'
    })
    if (!res.ok) {
      redirectToLogin()
      return
    }
    const data = await res.json()
    config.value = data.config

    // Ensure nested arrays exist so UI doesn't crash
    if (!config.value.llm) config.value.llm = {}
    if (!config.value.llm.providers) config.value.llm.providers = []
    if (!config.value.llm.embedding_providers) config.value.llm.embedding_providers = []
    if (!config.value.telegram) config.value.telegram = { bots: [], listen_addr: '' }
    if (!config.value.telegram.bots) config.value.telegram.bots = []
    if (!config.value.global) config.value.global = {}
    if (!config.value.agent_pool) config.value.agent_pool = {}

    // Ensure each bot has all required properties (providers are global, not per-bot)
    config.value.telegram.bots.forEach((bot: any) => {
      if (!bot.allowed_orchestrator_tools) bot.allowed_orchestrator_tools = []
      if (!bot.allowed_agent_tools) bot.allowed_agent_tools = []
      if (!bot.orchestrator_system_prompt) bot.orchestrator_system_prompt = ''
      if (!bot.agent_system_prompt) bot.agent_system_prompt = ''
      if (!bot.gp_model) bot.gp_model = ''
      if (!bot.reasoner_model) bot.reasoner_model = ''
    })

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
      credentials: 'include',
      body: JSON.stringify(config.value)
    })

    if (!res.ok) {
      const err = await res.json()
      document.getElementById('errorToastMessage')!.innerText = err.error || 'Failed to save configuration.'
      showToast('errorToast')
      if (res.status === 401 || res.status === 403) {
        redirectToLogin()
      }
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

const updateModels = (provider: any, val: string) => {
  provider.models = val.split(',').map((s: string) => s.trim()).filter((s: string) => s.length > 0)
}

const addLLMProvider = (listName: string) => {
  if (!config.value.llm[listName]) {
    config.value.llm[listName] = []
  }
  const defaultModels = listName === 'embedding_providers' ? ['text-embedding-3-large'] : ['gpt-4o']
  config.value.llm[listName].push({
    base_url: 'https://api.openai.com/v1',
    api_key: '',
    models: defaultModels
  })
}

const fetchTools = async () => {
  try {
    const res = await fetch('/api/v1/tools', {
      credentials: 'include'
    })
    if (!res.ok) {
      redirectToLogin()
      return
    }
    const data = await res.json()
    availableTools.value = data.tools || []
  } catch (err) {
    console.error('Failed to fetch tools', err)
  }
}

onMounted(async () => {
  isLoading.value = true

  // Check authentication first
  const authed = await checkAuth()
  if (!authed) {
    redirectToLogin()
    return
  }

  isAuthenticated.value = true
  fetchConfig()
  fetchHealth()
  fetchSessions()
  fetchTools()
  healthInterval = setInterval(fetchHealth, 5000)
})

onUnmounted(() => {
  if (healthInterval) clearInterval(healthInterval)
})
</script>
