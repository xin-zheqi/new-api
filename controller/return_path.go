package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func paymentReturnPath(suffix string) string {
	if common.GetTheme() == "classic" {
		switch {
		case suffix == "/wallet" || strings.HasPrefix(suffix, "/wallet?"):
			suffix = "/console/topup" + strings.TrimPrefix(suffix, "/wallet")
		case suffix == "/usage-logs" || strings.HasPrefix(suffix, "/usage-logs?"):
			suffix = "/console/log" + strings.TrimPrefix(suffix, "/usage-logs")
		}
	}
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + suffix
}
