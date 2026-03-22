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
	MaxTaskDepth int `mapstructure:"max_task_depth"`
	MaxQueueSize int `mapstructure:"max_queue_size"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

type ModelProviderConfig struct {
	BaseURL string   `mapstructure:"base_url"`
	APIKey  string   `mapstructure:"api_key"`
	Models  []string `mapstructure:"models"`
}

type LLMConfig struct {
	Providers          []ModelProviderConfig `mapstructure:"providers" json:"providers"`
	ReasonerProviders  []ModelProviderConfig `mapstructure:"reasoner_providers" json:"reasoner_providers"`
	EmbeddingProviders []ModelProviderConfig `mapstructure:"embedding_providers" json:"embedding_providers"`
	EmbeddingDims      int                   `mapstructure:"embedding_dims" json:"embedding_dims"`
	MaxTokens          int                   `mapstructure:"max_tokens" json:"max_tokens"`
	TimeoutSecs        int                   `mapstructure:"timeout_seconds" json:"timeout_seconds"`
	SystemPrompt       string                `mapstructure:"system_prompt" json:"system_prompt"`
	AgentSystemPrompt  string                `mapstructure:"agent_system_prompt" json:"agent_system_prompt"`
}

type AgentPoolConfig struct {
	MinAgents       int `mapstructure:"min_agents"`
	MaxAgents       int `mapstructure:"max_agents"`
	IdleTimeoutSecs int `mapstructure:"idle_timeout_seconds"`
}

type SnapshotConfig struct {
	Enabled           bool            `mapstructure:"enabled"`
	Retention         RetentionConfig `mapstructure:"retention"`
	StoragePath       string          `mapstructure:"storage_path"`
	MaxStorageBytes   int64           `mapstructure:"max_storage_bytes"`
}

type RetentionConfig struct {
	Completed  string `mapstructure:"COMPLETED"`
	Failed     string `mapstructure:"FAILED"`
	Exited     string `mapstructure:"EXITED"`
	Interrupted string `mapstructure:"INTERRUPTED"`
}

type TelegramConfig struct {
	BotToken  string `mapstructure:"bot_token"`
	WebhookURL string `mapstructure:"webhook_url"`
	ListenAddr string `mapstructure:"listen_addr"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type AuthConfig struct {
	JWTSecret          string `mapstructure:"jwt_secret"`
	JWTIssuer          string `mapstructure:"jwt_issuer"`
	AllowedMemberships []string `mapstructure:"allowed_memberships"`
}

type ContextConfig struct {
	MaxTokens            int     `mapstructure:"max_tokens"`
	CompactionThreshold  float64 `mapstructure:"compaction_threshold"`
	PreserveRecentTurns  int     `mapstructure:"preserve_recent_turns"`
	CompactionAgentType  string  `mapstructure:"compaction_agent_type"`
}

type RAGConfig struct {
	Enabled      bool               `mapstructure:"enabled"`
	DefaultTopK  int                `mapstructure:"default_top_k"`
	AutoRetrieve AutoRetrieveConfig `mapstructure:"auto_retrieve"`
	MinSimilarity float64          `mapstructure:"min_similarity"`
}

type AutoRetrieveConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	OnLLMCall   bool `mapstructure:"on_llm_call"`
	TopK        int  `mapstructure:"top_k"`
	MaxResults  int  `mapstructure:"max_results"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
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
	if c.Telegram.BotToken == "" {
		return fmt.Errorf("telegram.bot_token is required")
	}
	if c.Telegram.WebhookURL == "" {
		return fmt.Errorf("telegram.webhook_url is required")
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
