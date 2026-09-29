package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestClaudeToolResultToChatHoistsImages(t *testing.T) {
	text := "screenshot taken"
	content, media := claudeToolResultToChat([]dto.ClaudeMediaMessage{
		{Type: "text", Text: &text},
		{Type: "image", Source: &dto.ClaudeMessageSource{Type: "url", Url: "https://example.test/image.png"}},
	})

	assert.Equal(t, text, content)
	if assert.Len(t, media, 1) {
		assert.Equal(t, "image_url", media[0].Type)
		if image := media[0].GetImageMedia(); assert.NotNil(t, image) {
			assert.Equal(t, "https://example.test/image.png", image.Url)
		}
	}
}

func TestClaudeToolResultToChatUsesImagePlaceholder(t *testing.T) {
	content, media := claudeToolResultToChat([]dto.ClaudeMediaMessage{
		{Type: "image", Source: &dto.ClaudeMessageSource{MediaType: "image/png", Data: "abc"}},
	})

	assert.Equal(t, "[image]", content)
	if assert.Len(t, media, 1) {
		if image := media[0].GetImageMedia(); assert.NotNil(t, image) {
			assert.Equal(t, "data:image/png;base64,abc", image.Url)
		}
	}
}
