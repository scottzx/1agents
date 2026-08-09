package main

import (
	// cmd/ may import app root packages to wire them into the binary.
	_ "github.com/scottzx/1Agents/backend/internal/apps/alpha"
	// cmd/ may also use the aggregator.
	_ "github.com/scottzx/1Agents/backend/internal/apps"
)

func main() {}
