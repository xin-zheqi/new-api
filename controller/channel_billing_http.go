package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

const maxChannelBalanceResponseBytes = 256 << 10

// Management balance queries use the channel proxy and bounded response bodies.
// Callers supply a finite context deadline, separate from streaming relay limits.
func getChannelBalanceResponse(ctx context.Context, channel *model.Channel, requestURL string, headers http.Header, provider string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, channel.Key)
	}
	request.Header = headers.Clone()
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, channel.Key)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, sanitizeFetchModelsError(err, channel.Key)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s balance query returned HTTP %d", provider, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxChannelBalanceResponseBytes+1))
	if err != nil {
		return nil, sanitizeFetchModelsError(err, channel.Key)
	}
	if len(body) > maxChannelBalanceResponseBytes {
		return nil, fmt.Errorf("%s balance response is too large", provider)
	}
	return body, nil
}
