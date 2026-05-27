package tools

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/genai"

	"github.com/thamwangjun/generate-visuals-mcp/internal/config"
)

const geminiModel = "gemini-3.1-flash-image-preview"

// Register adds the generate_visuals tool to the MCP server.
func Register(s *server.MCPServer, cfg *config.Config) {
	tool := mcp.NewTool(
		"generate_visuals",
		mcp.WithDescription("Generate an image from a text prompt using the Gemini image generation model."),
		mcp.WithTitleAnnotation("Generate Visual"),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithString("image_prompt",
			mcp.Required(),
			mcp.Description("A descriptive text prompt for the image to generate."),
		),
	)

	s.AddTool(tool, MakeGenerateVisualsHandler(cfg))
}

// MakeGenerateVisualsHandler returns a ToolHandlerFunc that generates images via Gemini.
// Exported so tests can call it directly.
func MakeGenerateVisualsHandler(cfg *config.Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result *mcp.CallToolResult

		func() {
			defer func() {
				if r := recover(); r != nil {
					result = mcp.NewToolResultError(fmt.Sprintf("internal panic recovered: %v", r))
				}
			}()
			result = DoGenerateVisuals(ctx, cfg, req)
		}()

		return result, nil
	}
}

// DoGenerateVisuals performs the actual image generation. Exported for test access.
func DoGenerateVisuals(ctx context.Context, cfg *config.Config, req mcp.CallToolRequest) *mcp.CallToolResult {
	prompt, err := req.RequireString("image_prompt")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing or invalid image_prompt parameter: %v", err))
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"failed to create Gemini client: %v. Likely cause: invalid API key or network error.", err,
		))
	}

	resp, err := client.Models.GenerateContent(
		ctx,
		geminiModel,
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			ResponseModalities: []string{
				string(genai.ModalityText),
				string(genai.ModalityImage),
			},
		},
	)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"Gemini API call failed: %v. Likely cause: model unavailable, quota exceeded, or network error.", err,
		))
	}

	if len(resp.Candidates) == 0 {
		return mcp.NewToolResultError("D-07: no candidates returned by the model; content may have been filtered")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return mcp.NewToolResultError("D-07: candidate returned with no content parts")
	}

	for _, part := range candidate.Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			mimeType := part.InlineData.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			imgBase64 := base64.StdEncoding.EncodeToString(part.InlineData.Data)
			return mcp.NewToolResultImage("", imgBase64, mimeType)
		}
	}

	return mcp.NewToolResultError("D-07: no image data found in Gemini response parts")
}
