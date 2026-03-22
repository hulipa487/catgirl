package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	DefaultConfigPath = "/etc/catgirl.conf"
)

type Config struct {
	Database      DatabaseConfig `mapstructure:"database"`
	Server        ServerConfig   `mapstructure:"server"`
	Logging       LoggingConfig  `mapstructure:"logging"`

	// These values act as defaults seeded into the DB on first run
	RuntimeSeed   RuntimeConfig  `mapstructure:",squash"`
}

// RuntimeConfig represents all dynamically reloadable config stored in the DB
type RuntimeConfig struct {
	Global      GlobalConfig      `json:"global" mapstructure:"global"`
	LLM         LLMConfig         `json:"llm" mapstructure:"llm"`
	AgentPool   AgentPoolConfig   `json:"agent_pool" mapstructure:"agent_pool"`
	Snapshot    SnapshotConfig     `json:"snapshots" mapstructure:"snapshots"`
	Telegram    TelegramConfig    `json:"telegram" mapstructure:"telegram"`
	Auth        AuthConfig        `json:"auth" mapstructure:"auth"`
	Context     ContextConfig     `json:"context" mapstructure:"context"`
	RAG         RAGConfig         `json:"rag" mapstructure:"rag"`
}

type GlobalConfig struct {
	MaxTaskDepth int `mapstructure:"max_task_depth" json:"max_task_depth"`
	MaxQueueSize int `mapstructure:"max_queue_size" json:"max_queue_size"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host" json:"host"`
	Port     int    `mapstructure:"port" json:"port"`
	User     string `mapstructure:"user" json:"user"`
	Password string `mapstructure:"password" json:"password"`
	DBName   string `mapstructure:"dbname" json:"dbname"`
	SSLMode  string `mapstructure:"sslmode" json:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

type ModelProviderConfig struct {
	BaseURL string   `mapstructure:"base_url" json:"base_url"`
	APIKey  string   `mapstructure:"api_key" json:"api_key"`
	Models  []string `mapstructure:"models" json:"models"`
}

type LLMConfig struct {
	Providers          []ModelProviderConfig `mapstructure:"providers" json:"providers"`
	ReasonerProviders  []ModelProviderConfig `mapstructure:"reasoner_providers" json:"reasoner_providers"`
	EmbeddingProviders []ModelProviderConfig `mapstructure:"embedding_providers" json:"embedding_providers"`
	EmbeddingDims      int                   `mapstructure:"embedding_dims" json:"embedding_dims"`
	MaxTokens          int                   `mapstructure:"max_tokens" json:"max_tokens"`
	TimeoutSecs        int                   `mapstructure:"timeout_seconds" json:"timeout_seconds"`

	// Session Defaults
	DefaultSystemPrompt      string   `mapstructure:"default_system_prompt" json:"default_system_prompt"`
	DefaultAgentSystemPrompt string   `mapstructure:"default_agent_system_prompt" json:"default_agent_system_prompt"`
	DefaultOrchestratorTools []string `mapstructure:"default_orchestrator_tools" json:"default_orchestrator_tools"`
	DefaultAgentTools        []string `mapstructure:"default_agent_tools" json:"default_agent_tools"`
}

type AgentPoolConfig struct {
	MinAgents       int `mapstructure:"min_agents" json:"min_agents"`
	MaxAgents       int `mapstructure:"max_agents" json:"max_agents"`
	IdleTimeoutSecs int `mapstructure:"idle_timeout_seconds" json:"idle_timeout_seconds"`
}

type SnapshotConfig struct {
	Enabled         bool            `mapstructure:"enabled" json:"enabled"`
	Retention       RetentionConfig `mapstructure:"retention" json:"retention"`
	StoragePath     string          `mapstructure:"storage_path" json:"storage_path"`
	MaxStorageBytes int64           `mapstructure:"max_storage_bytes" json:"max_storage_bytes"`
}

type RetentionConfig struct {
	Completed   string `mapstructure:"COMPLETED" json:"COMPLETED"`
	Failed      string `mapstructure:"FAILED" json:"FAILED"`
	Exited      string `mapstructure:"EXITED" json:"EXITED"`
	Interrupted string `mapstructure:"INTERRUPTED" json:"INTERRUPTED"`
}

type TelegramBotConfig struct {
	BotToken                 string   `mapstructure:"bot_token" json:"bot_token"`
	WebhookURL               string   `mapstructure:"webhook_url" json:"webhook_url"`
	OrchestratorSystemPrompt string   `mapstructure:"orchestrator_system_prompt" json:"orchestrator_system_prompt"`
	AgentSystemPrompt        string   `mapstructure:"agent_system_prompt" json:"agent_system_prompt"`
	AllowedOrchestratorTools []string `mapstructure:"allowed_orchestrator_tools" json:"allowed_orchestrator_tools"`
	AllowedAgentTools        []string `mapstructure:"allowed_agent_tools" json:"allowed_agent_tools"`
	GPModel                  string   `mapstructure:"gp_model" json:"gp_model"`
	ReasonerModel            string   `mapstructure:"reasoner_model" json:"reasoner_model"`
}

type TelegramConfig struct {
	Bots       []TelegramBotConfig `mapstructure:"bots" json:"bots"`
	ListenAddr string              `mapstructure:"listen_addr" json:"listen_addr"`
}

type ServerConfig struct {
	Host string `mapstructure:"host" json:"host"`
	Port int    `mapstructure:"port" json:"port"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type AuthConfig struct {
	JWTSecret          string   `mapstructure:"jwt_secret" json:"jwt_secret"`
	JWTIssuer          string   `mapstructure:"jwt_issuer" json:"jwt_issuer"`
	AllowedMemberships []string `mapstructure:"allowed_memberships" json:"allowed_memberships"`
}

type ContextConfig struct {
	MaxTokens           int     `mapstructure:"max_tokens" json:"max_tokens"`
	CompactionThreshold float64 `mapstructure:"compaction_threshold" json:"compaction_threshold"`
	PreserveRecentTurns int     `mapstructure:"preserve_recent_turns" json:"preserve_recent_turns"`
	CompactionAgentType string  `mapstructure:"compaction_agent_type" json:"compaction_agent_type"`
}

type RAGConfig struct {
	Enabled       bool               `mapstructure:"enabled" json:"enabled"`
	DefaultTopK   int                `mapstructure:"default_top_k" json:"default_top_k"`
	AutoRetrieve  AutoRetrieveConfig `mapstructure:"auto_retrieve" json:"auto_retrieve"`
	MinSimilarity float64            `mapstructure:"min_similarity" json:"min_similarity"`
}

type AutoRetrieveConfig struct {
	Enabled    bool `mapstructure:"enabled" json:"enabled"`
	OnLLMCall  bool `mapstructure:"on_llm_call" json:"on_llm_call"`
	TopK       int  `mapstructure:"top_k" json:"top_k"`
	MaxResults int  `mapstructure:"max_results" json:"max_results"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level" json:"level"`
	Format string `mapstructure:"format" json:"format"`
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	if configPath == "" {
		configPath = DefaultConfigPath
	}

	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Validate the initial seed
	if err := cfg.RuntimeSeed.Validate(); err != nil {
		return nil, fmt.Errorf("runtime config seed validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	return nil
}

func (c *RuntimeConfig) Validate() error {
	if len(c.LLM.Providers) == 0 {
		return fmt.Errorf("at least one llm.providers entry is required")
	}
	for i, p := range c.LLM.Providers {
		if len(p.Models) == 0 {
			return fmt.Errorf("llm.providers[%d] requires at least one model", i)
		}
	}
	// Note: reasoner_providers is optional, but if specified, must have at least one model
	for i, p := range c.LLM.ReasonerProviders {
		if len(p.Models) == 0 {
			return fmt.Errorf("llm.reasoner_providers[%d] requires at least one model", i)
		}
	}
	if len(c.LLM.EmbeddingProviders) == 0 {
		return fmt.Errorf("at least one llm.embedding_providers entry is required")
	}
	for i, p := range c.LLM.EmbeddingProviders {
		if len(p.Models) == 0 {
			return fmt.Errorf("llm.embedding_providers[%d] requires at least one model", i)
		}
	}
	for i, b := range c.Telegram.Bots {
		if b.BotToken == "" {
			return fmt.Errorf("telegram.bots[%d].bot_token is required", i)
		}
		if b.WebhookURL == "" {
			return fmt.Errorf("telegram.bots[%d].webhook_url is required", i)
		}
	}
	if c.Global.MaxTaskDepth == 0 {
		c.Global.MaxTaskDepth = 3
	}
	if c.Global.MaxQueueSize == 0 {
		c.Global.MaxQueueSize = 1000
	}
	if c.AgentPool.MaxAgents == 0 {
		c.AgentPool.MaxAgents = 50
	}
	if c.AgentPool.MinAgents == 0 {
		c.AgentPool.MinAgents = 5
	}
	if c.Context.MaxTokens == 0 {
		c.Context.MaxTokens = 128000
	}
	if c.Context.CompactionThreshold == 0 {
		c.Context.CompactionThreshold = 0.8
	}
	if c.Context.PreserveRecentTurns == 0 {
		c.Context.PreserveRecentTurns = 20
	}
	if c.Context.CompactionAgentType == "" {
		c.Context.CompactionAgentType = "reasoner"
	}
	if c.RAG.DefaultTopK == 0 {
		c.RAG.DefaultTopK = 5
	}
	if c.RAG.AutoRetrieve.TopK == 0 {
		c.RAG.AutoRetrieve.TopK = 3
	}
	if c.RAG.AutoRetrieve.MaxResults == 0 {
		c.RAG.AutoRetrieve.MaxResults = 10
	}
	if c.LLM.EmbeddingDims == 0 {
		c.LLM.EmbeddingDims = 1024
	}
	return nil
}

func ConfigFlagersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("config", "c", DefaultConfigPath, "Path to configuration file")
}

func GetConfigPath(cmd *cobra.Command) string {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return DefaultConfigPath
	}
	if configPath == "" {
		return DefaultConfigPath
	}
	if !filepath.IsAbs(configPath) {
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return configPath
		}
		return absPath
	}
	return configPath
}
