package main

import (
	"go.uber.org/zap"
)

type Config struct {
	Logger *zap.SugaredLogger
}

func NewConfig() (*Config, error) {
	logger, err := NewLogger()
	if err != nil {
		return nil, err
	}

	return &Config{
		Logger: logger,
	}, nil
}
