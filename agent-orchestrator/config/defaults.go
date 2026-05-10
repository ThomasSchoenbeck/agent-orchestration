package config

const (
	DefaultServerPort               = 8080
	DefaultServerHost               = "0.0.0.0"
	DefaultDBType                   = "sqlite"
	DefaultDBPath                   = "./data/orchestrator.db"
	DefaultHeartbeatIntervalSec     = 30
	DefaultTaskPollIntervalSec      = 5
	DefaultTaskTimeoutSec           = 300
	DefaultMaxRetries               = 3
	DefaultCircuitBreakerThreshold  = 5
)
