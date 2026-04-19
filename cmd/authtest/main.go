// authtest: интерактивный тест device code flow для OpenAI/Codex.
// Запускать: go run ./cmd/authtest
// Не является частью основного бинарника.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

const tokenCacheFile = "authtest_token.json"

func main() {
	cfg := auth.OpenAIOAuthConfig()

	cred := loadCachedToken()
	if cred != nil {
		fmt.Printf("Используем кешированный токен (AccountID: %s)\n", cred.AccountID)
		if cred.NeedsRefresh() && cred.RefreshToken != "" {
			fmt.Println("Токен истекает, обновляем...")
			refreshed, err := auth.RefreshAccessToken(cred, cfg)
			if err != nil {
				fmt.Printf("Ошибка обновления токена: %v\nПроходим логин заново...\n\n", err)
				cred = nil
			} else {
				cred = refreshed
				saveCachedToken(cred)
				fmt.Println("Токен обновлён.")
			}
		}
	}

	if cred == nil {
		fmt.Println("=== OpenAI Device Code Login ===")
		fmt.Printf("Issuer:   %s\n", cfg.Issuer)
		fmt.Printf("ClientID: %s\n\n", cfg.ClientID)

		info, err := auth.RequestDeviceCode(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка RequestDeviceCode: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Откройте в браузере:\n  %s\n\nКод для ввода: %s\n\n", info.VerifyURL, info.UserCode)
		fmt.Printf("Polling interval: %d сек\nОжидаем подтверждения...\n", info.Interval)

		deadline := time.After(15 * time.Minute)
		ticker := time.NewTicker(time.Duration(info.Interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-deadline:
				fmt.Fprintln(os.Stderr, "Таймаут 15 минут")
				os.Exit(1)
			case <-ticker.C:
				c, err := auth.PollDeviceCodeOnce(cfg, info.DeviceAuthID, info.UserCode)
				if err != nil {
					continue
				}
				if c != nil {
					cred = c
					goto done
				}
			}
		}
	done:
		saveCachedToken(cred)
		fmt.Println("\n=== Токен получен! ===")
		fmt.Printf("AccountID:  %q\n", cred.AccountID)
		fmt.Printf("ExpiresAt:  %s\n", cred.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("AccessToken: %s\n\n", maskToken(cred.AccessToken))
		fmt.Println("=== JWT Claims (AccessToken) ===")
		printJWTClaims(cred.AccessToken)
	}

	fmt.Println("\n=== Тест вызова Codex API (через SDK) ===")
	callCodexViaSDK(cred.AccessToken, cred.AccountID)

	fmt.Println("\n=== Тест tool calling ===")
	callCodexWithTool(cred.AccessToken, cred.AccountID)

	fmt.Println("\n=== Raw ответ от API (дамп Output items) ===")
	dumpRawResponse(cred.AccessToken, cred.AccountID)
}

func loadCachedToken() *auth.AuthCredential {
	data, err := os.ReadFile(tokenCacheFile)
	if err != nil {
		return nil
	}
	var cred auth.AuthCredential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil
	}
	if cred.AccessToken == "" {
		return nil
	}
	if cred.IsExpired() && cred.RefreshToken == "" {
		fmt.Println("Кешированный токен истёк и нет refresh token, логинимся заново.")
		return nil
	}
	return &cred
}

func saveCachedToken(cred *auth.AuthCredential) {
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		fmt.Printf("Ошибка сериализации токена: %v\n", err)
		return
	}
	if err := os.WriteFile(tokenCacheFile, data, 0600); err != nil {
		fmt.Printf("Ошибка сохранения токена: %v\n", err)
		return
	}
	fmt.Printf("Токен сохранён в %s\n", tokenCacheFile)
}

func callCodexViaSDK(token, accountID string) {
	p := providers.NewCodexProvider(token, accountID)

	msgs := []protocoltypes.Message{
		{Role: "user", Content: "Say exactly: hello"},
	}

	fmt.Printf("Вызываем CodexProvider.Chat() с моделью gpt-5.4...\n\n")

	resp, err := p.Chat(context.Background(), msgs, nil, "gpt-5.4", nil)
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		return
	}

	fmt.Printf("OK!\n")
	fmt.Printf("FinishReason:     %s\n", resp.FinishReason)
	fmt.Printf("Content:          %q\n", resp.Content)
	fmt.Printf("ReasoningContent: %q\n", resp.ReasoningContent)
	fmt.Printf("ToolCalls:        %d\n", len(resp.ToolCalls))
	if resp.Usage != nil {
		fmt.Printf("Usage: prompt=%d completion=%d total=%d\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
}

func callCodexWithTool(token, accountID string) {
	p := providers.NewCodexProvider(token, accountID)

	tools := []protocoltypes.ToolDefinition{
		{
			Type: "function",
			Function: protocoltypes.ToolFunctionDefinition{
				Name:        "get_weather",
				Description: "Get current weather for a city",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string", "description": "City name"},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	msgs := []protocoltypes.Message{
		{Role: "user", Content: "What is the weather in Moscow right now?"},
	}

	fmt.Println("Вызываем CodexProvider.Chat() с tool get_weather, модель gpt-5.3-codex...")

	resp, err := p.Chat(context.Background(), msgs, tools, "gpt-5.3-codex", nil)
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		return
	}

	fmt.Printf("FinishReason: %s\n", resp.FinishReason)
	fmt.Printf("Content:      %q\n", resp.Content)
	fmt.Printf("ToolCalls:    %d\n", len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		args, _ := json.MarshalIndent(tc.Arguments, "    ", "  ")
		fmt.Printf("  [%d] id=%s name=%s args=%s\n", i, tc.ID, tc.Name, args)
	}
}

func dumpRawResponse(token, accountID string) {
	client := openai.NewClient(
		option.WithBaseURL("https://chatgpt.com/backend-api/codex"),
		option.WithAPIKey(token),
		option.WithHeader("originator", "codex_cli_rs"),
		option.WithHeader("OpenAI-Beta", "responses=experimental"),
		option.WithHeader("Chatgpt-Account-Id", accountID),
	)

	params := responses.ResponseNewParams{
		Model: "gpt-5.4",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{OfMessage: &responses.EasyInputMessageParam{
					Role:    responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{OfString: openai.Opt("Say exactly: hello")},
				}},
			},
		},
		Instructions: openai.Opt("You are Codex, a coding assistant."),
		Store:        openai.Opt(false),
	}

	stream := client.Responses.NewStreaming(context.Background(), params)
	defer stream.Close()

	fmt.Println("Stream events:")
	for stream.Next() {
		evt := stream.Current()
		switch evt.Type {
		case "response.completed", "response.failed":
			fmt.Printf("\n[%s] Response.Output len=%d\n", evt.Type, len(evt.Response.Output))
			out, _ := json.MarshalIndent(evt.Response.Output, "", "  ")
			fmt.Printf("Output items: %s\n", out)
		case "response.output_item.done":
			fmt.Printf("\n[response.output_item.done]\n")
			raw, _ := json.MarshalIndent(evt.Item, "", "  ")
			fmt.Printf("Item: %s\n", raw)
		case "response.output_text.delta":
			fmt.Printf("  [delta] %q\n", evt.Delta)
		case "response.output_text.done":
			fmt.Printf("  [text.done] %q\n", evt.Text)
		default:
			fmt.Printf("  event: %s\n", evt.Type)
		}
	}
	if err := stream.Err(); err != nil {
		fmt.Printf("Stream error: %v\n", err)
	}
}

func maskToken(t string) string {
	if len(t) <= 12 {
		return strings.Repeat("*", len(t))
	}
	return t[:6] + "..." + t[len(t)-6:]
}

func printJWTClaims(token string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		fmt.Println("  (не JWT, непрозрачный токен)")
		return
	}

	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	payload = strings.NewReplacer("-", "+", "_", "/").Replace(payload)

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		fmt.Printf("  ошибка декодирования base64: %v\n", err)
		return
	}

	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		fmt.Printf("  ошибка разбора JSON: %v\n", err)
		return
	}

	out, _ := json.MarshalIndent(claims, "  ", "  ")
	fmt.Println(string(out))
}
