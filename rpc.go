package jsmachine

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// rpc represents the RPC interface exposed to PHP
type rpc struct {
	plugin *Plugin
	log    *zap.Logger
}

// ExecuteRequest represents a JavaScript execution request from PHP
type ExecuteRequest struct {
	// JavaScript code to execute
	Code string `json:"code"`

	// Execution timeout in milliseconds (0 = use default)
	TimeoutMs int `json:"timeout_ms"`

	// Request context for logging/tracing
	RequestID string `json:"request_id,omitempty"`
}

// ExecuteResponse represents the execution result
type ExecuteResponse struct {
	// Execution result (can be any JSON-serializable type)
	Result interface{} `json:"result"`

	// Execution duration in milliseconds
	DurationMs int64 `json:"duration_ms"`

	// Error message if execution failed
	Error string `json:"error,omitempty"`

	// Request ID for correlation
	RequestID string `json:"request_id,omitempty"`
}

// Execute runs JavaScript code and returns the result
func (r *rpc) Execute(req *ExecuteRequest, resp *ExecuteResponse) error {
	start := time.Now()

	// Validate request
	if req.Code == "" {
		resp.Error = "code is required"
		return fmt.Errorf("code is required")
	}

	// Determine timeout
	timeout := time.Duration(r.plugin.cfg.DefaultTimeout) * time.Millisecond
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	// Log execution start
	r.log.Debug("executing JavaScript",
		zap.String("request_id", req.RequestID),
		zap.Int("code_length", len(req.Code)),
		zap.Duration("timeout", timeout),
	)

	// Execute JavaScript with background context
	ctx := context.Background()
	result, err := r.plugin.execute(ctx, req.Code, timeout)

	duration := time.Since(start)
	resp.DurationMs = duration.Milliseconds()
	resp.RequestID = req.RequestID

	if err != nil {
		resp.Error = err.Error()
		r.log.Error("JavaScript execution failed",
			zap.String("request_id", req.RequestID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		return nil // Don't return error to RPC, encode it in response
	}

	resp.Result = result

	r.log.Debug("JavaScript execution completed",
		zap.String("request_id", req.RequestID),
		zap.Duration("duration", duration),
	)

	return nil
}
