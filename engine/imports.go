// Package engine imports required dependencies for the ForkSync engine.
// This file ensures go.mod retains the required dependencies.
package main

import (
	_ "github.com/go-git/go-git/v5"
	_ "github.com/google/uuid"
	_ "github.com/sashabaranov/go-openai"
	_ "github.com/spf13/cobra"
	_ "github.com/spf13/viper"
	_ "modernc.org/sqlite"
)
