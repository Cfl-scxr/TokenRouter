package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// ReplaceModelMetadata 只替换常见协议中的模型元数据字段，不触碰正文内容。
func ReplaceModelMetadata(data []byte, fromModel, toModel string) []byte {
	fromModel = strings.TrimSpace(fromModel)
	toModel = strings.TrimSpace(toModel)
	if fromModel == "" || toModel == "" || fromModel == toModel {
		return data
	}
	fromValue, _ := json.Marshal(fromModel)
	toValue, _ := json.Marshal(toModel)
	fromGeminiName, _ := json.Marshal("models/" + fromModel)
	toGeminiName, _ := json.Marshal("models/" + toModel)
	patterns := [][2][]byte{
		{append([]byte(`"model":`), fromValue...), append([]byte(`"model":`), toValue...)},
		{append([]byte(`"model": `), fromValue...), append([]byte(`"model": `), toValue...)},
		{append([]byte(`"modelVersion":`), fromValue...), append([]byte(`"modelVersion":`), toValue...)},
		{append([]byte(`"modelVersion": `), fromValue...), append([]byte(`"modelVersion": `), toValue...)},
		{append([]byte(`"model_version":`), fromValue...), append([]byte(`"model_version":`), toValue...)},
		{append([]byte(`"model_version": `), fromValue...), append([]byte(`"model_version": `), toValue...)},
		{append([]byte(`"id":`), fromValue...), append([]byte(`"id":`), toValue...)},
		{append([]byte(`"id": `), fromValue...), append([]byte(`"id": `), toValue...)},
		{append([]byte(`"name":`), fromGeminiName...), append([]byte(`"name":`), toGeminiName...)},
		{append([]byte(`"name": `), fromGeminiName...), append([]byte(`"name": `), toGeminiName...)},
	}
	rewritten := data
	for _, pattern := range patterns {
		rewritten = bytes.ReplaceAll(rewritten, pattern[0], pattern[1])
	}
	return rewritten
}

// RestoreAPIKeyModelResponse 按请求追踪恢复 Key 重定向产生的内部模型名。
func RestoreAPIKeyModelResponse(ctx context.Context, data []byte) []byte {
	trace, ok := APIKeyModelRedirectTraceFromContext(ctx)
	if !ok || strings.TrimSpace(trace.ClientModel) == "" {
		return data
	}
	rewritten := data
	for _, model := range trace.ResponseModels() {
		rewritten = ReplaceModelMetadata(rewritten, model, trace.ClientModel)
	}
	return rewritten
}
