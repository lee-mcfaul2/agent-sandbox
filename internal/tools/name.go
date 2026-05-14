package tools

import (
	"errors"
	"strings"
)

const Separator = "__"

func EncodeName(mcp, tool string) string {
	return mcp + Separator + tool
}

func DecodeName(name string) (mcp, tool string, err error) {
	idx := strings.Index(name, Separator)
	if idx < 0 {
		return "", "", errors.New("tool name missing __ separator")
	}
	mcp = name[:idx]
	tool = name[idx+len(Separator):]
	if mcp == "" {
		return "", "", errors.New("tool name has empty mcp")
	}
	if tool == "" {
		return "", "", errors.New("tool name has empty tool")
	}
	return mcp, tool, nil
}
