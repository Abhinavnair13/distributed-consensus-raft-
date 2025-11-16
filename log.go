// Filename: log.go
package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates and returns a new key:value based sugared logger.
func NewLogger() (*zap.SugaredLogger, error) {
	// We'll build a "Development" logger for a good developer experience.
	// This includes colored levels and key-value pairs.
	config := zap.NewDevelopmentConfig()

	// Customize the encoder to be key-value based and colorized
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Build the logger
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	// Return the sugared logger, which is easier to use.
	return logger.Sugar(), nil
}
