package service

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"go.uber.org/atomic"
)

var (
	p                 *xray.Process
	lock              sync.Mutex
	isNeedXrayRestart atomic.Bool // Indicates that restart was requested for Xray
	isManuallyStopped atomic.Bool // Indicates that Xray was stopped manually from the panel
	result            string
)

// XrayService provides business logic for Xray process management.
// It handles starting, stopping, restarting Xray, and managing its configuration.
type XrayService struct {
	inboundService InboundService
	settingService SettingService
	xrayAPI        xray.XrayAPI
}

// IsXrayRunning checks if the Xray process is currently running.
func (s *XrayService) IsXrayRunning() bool {
	return p != nil && p.IsRunning()
}

// GetXrayErr returns the error from the Xray process, if any.
func (s *XrayService) GetXrayErr() error {
	if p == nil {
		return nil
	}

	err := p.GetErr()

	if runtime.GOOS == "windows" && err.Error() == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return nil
	}

	return err
}

// GetXrayResult returns the result string from the Xray process.
func (s *XrayService) GetXrayResult() string {
	if result != "" {
		return result
	}
	if s.IsXrayRunning() {
		return ""
	}
	if p == nil {
		return ""
	}

	result = p.GetResult()

	if runtime.GOOS == "windows" && result == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return ""
	}

	return result
}

// GetXrayVersion returns the version of the running Xray process.
func (s *XrayService) GetXrayVersion() string {
	if p == nil {
		return "Unknown"
	}
	return p.GetVersion()
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig retrieves and builds the Xray configuration from settings and inbounds.
func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}

	var templateOutbounds []map[string]any
	if len(xrayConfig.OutboundConfigs) > 0 {
		if err := json.Unmarshal(xrayConfig.OutboundConfigs, &templateOutbounds); err != nil {
			return nil, err
		}
	}
	if templateOutbounds == nil {
		templateOutbounds = []map[string]any{}
	}

	routingConfig := map[string]any{}
	if len(xrayConfig.RouterConfig) > 0 {
		if err := json.Unmarshal(xrayConfig.RouterConfig, &routingConfig); err != nil {
			return nil, err
		}
	}
	existingRules, _ := routingConfig["rules"].([]any)
	if existingRules == nil {
		existingRules = []any{}
	}

	desiredTorOutbounds := make(map[string]map[string]any)
	desiredTorRules := make(map[string]map[string]any)
	var desiredTorRuleOrder []string

	s.inboundService.AddTraffic(nil, nil)

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		clientModels, err := s.inboundService.GetClients(inbound)
		if err != nil {
			return nil, err
		}
		proxySettings := model.ParseProxySettings(inbound.ProxySettings)
		if proxySettings.IsTorPerClientEnabled() && inbound.Protocol == model.VLESS {
			creds, err := s.inboundService.GetTorCredentialsForInbound(inbound.Id)
			if err != nil {
				return nil, err
			}
			credMap := make(map[string]model.TorCredential, len(creds))
			for _, cred := range creds {
				credMap[strings.ToLower(cred.ClientEmail)] = cred
			}
			for _, client := range clientModels {
				emailKey := strings.ToLower(client.Email)
				if emailKey == "" {
					continue
				}
				cred, ok := credMap[emailKey]
				if !ok || cred.Username == "" {
					continue
				}
				tag := buildTorOutboundTag(inbound.Tag, client.Email)
				desiredTorOutbounds[tag] = map[string]any{
					"tag":      tag,
					"protocol": "socks",
					"settings": map[string]any{
						"servers": []any{
							map[string]any{
								"address": config.GetTorSocksAddress(),
								"port":    config.GetTorSocksPort(),
								"users": []any{
									map[string]any{
										"user": cred.Username,
										"pass": cred.Password,
									},
								},
							},
						},
					},
				}
				ruleKey := inbound.Tag + "|" + emailKey
				if _, exists := desiredTorRules[ruleKey]; !exists {
					desiredTorRules[ruleKey] = map[string]any{
						"type":        "field",
						"inboundTag":  []any{inbound.Tag},
						"user":        []any{client.Email},
						"outboundTag": tag,
					}
					desiredTorRuleOrder = append(desiredTorRuleOrder, ruleKey)
				}
			}
		}
		// get settings clients
		settings := map[string]any{}
		json.Unmarshal([]byte(inbound.Settings), &settings)
		clients, ok := settings["clients"].([]any)
		if ok {
			// check users active or not
			clientStats := inbound.ClientStats
			for _, clientTraffic := range clientStats {
				indexDecrease := 0
				for index, client := range clients {
					c := client.(map[string]any)
					if c["email"] == clientTraffic.Email {
						if !clientTraffic.Enable {
							clients = RemoveIndex(clients, index-indexDecrease)
							indexDecrease++
							logger.Infof("Remove Inbound User %s due to expiration or traffic limit", c["email"])
						}
					}
				}
			}

			// clear client config for additional parameters
			var final_clients []any
			for _, client := range clients {
				c := client.(map[string]any)
				if c["enable"] != nil {
					if enable, ok := c["enable"].(bool); ok && !enable {
						continue
					}
				}
				for key := range c {
					if key != "email" && key != "id" && key != "password" && key != "flow" && key != "method" {
						delete(c, key)
					}
					if c["flow"] == "xtls-rprx-vision-udp443" {
						c["flow"] = "xtls-rprx-vision"
					}
				}
				final_clients = append(final_clients, any(c))
			}

			settings["clients"] = final_clients
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}

			inbound.Settings = string(modifiedSettings)
		}

		if len(inbound.StreamSettings) > 0 {
			// Unmarshal stream JSON
			var stream map[string]any
			json.Unmarshal([]byte(inbound.StreamSettings), &stream)

			// Remove the "settings" field under "tlsSettings" and "realitySettings"
			tlsSettings, ok1 := stream["tlsSettings"].(map[string]any)
			realitySettings, ok2 := stream["realitySettings"].(map[string]any)
			if ok1 || ok2 {
				if ok1 {
					delete(tlsSettings, "settings")
				} else if ok2 {
					delete(realitySettings, "settings")
				}
			}

			delete(stream, "externalProxy")

			newStream, err := json.MarshalIndent(stream, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.StreamSettings = string(newStream)
		}

		inboundConfig := inbound.GenXrayInboundConfig()
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
	}

	filteredOutbounds := make([]map[string]any, 0, len(templateOutbounds)+len(desiredTorOutbounds))
	for _, outbound := range templateOutbounds {
		tag, _ := outbound["tag"].(string)
		if strings.HasPrefix(tag, torOutboundPrefix) {
			if cfg, ok := desiredTorOutbounds[tag]; ok {
				filteredOutbounds = append(filteredOutbounds, cfg)
				delete(desiredTorOutbounds, tag)
			}
			continue
		}
		filteredOutbounds = append(filteredOutbounds, outbound)
	}
	for _, outbound := range desiredTorOutbounds {
		filteredOutbounds = append(filteredOutbounds, outbound)
	}

	outBytes, err := json.MarshalIndent(filteredOutbounds, "", "  ")
	if err != nil {
		return nil, err
	}
	xrayConfig.OutboundConfigs = outBytes

	filteredRules := make([]any, 0, len(existingRules)+len(desiredTorRuleOrder))
	for _, rawRule := range existingRules {
		ruleMap, ok := rawRule.(map[string]any)
		if !ok {
			filteredRules = append(filteredRules, rawRule)
			continue
		}
		outboundTag, _ := ruleMap["outboundTag"].(string)
		if strings.HasPrefix(outboundTag, torOutboundPrefix) {
			continue
		}
		filteredRules = append(filteredRules, rawRule)
	}
	for _, key := range desiredTorRuleOrder {
		filteredRules = append(filteredRules, desiredTorRules[key])
	}
	routingConfig["rules"] = filteredRules
	routeBytes, err := json.MarshalIndent(routingConfig, "", "  ")
	if err != nil {
		return nil, err
	}
	xrayConfig.RouterConfig = routeBytes

	return xrayConfig, nil
}

const torOutboundPrefix = "tor-up-"

var inboundTagSanitizer = strings.NewReplacer(":", "-", " ", "-", ".", "-", "/", "-")

func buildTorOutboundTag(inboundTag, email string) string {
	sanitizedTag := inboundTagSanitizer.Replace(strings.ToLower(inboundTag))
	hash := sha1.Sum([]byte(strings.ToLower(email)))
	return fmt.Sprintf("%s%s-%x", torOutboundPrefix, sanitizedTag, hash[:4])
}

// GetXrayTraffic fetches the current traffic statistics from the running Xray process.
func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, []*xray.ClientTraffic, error) {
	if !s.IsXrayRunning() {
		err := errors.New("xray is not running")
		logger.Debug("Attempted to fetch Xray traffic, but Xray is not running:", err)
		return nil, nil, err
	}
	apiPort := p.GetAPIPort()
	s.xrayAPI.Init(apiPort)
	defer s.xrayAPI.Close()

	traffic, clientTraffic, err := s.xrayAPI.GetTraffic(true)
	if err != nil {
		logger.Debug("Failed to fetch Xray traffic:", err)
		return nil, nil, err
	}
	return traffic, clientTraffic, nil
}

// RestartXray restarts the Xray process, optionally forcing a restart even if config unchanged.
func (s *XrayService) RestartXray(isForce bool) error {
	lock.Lock()
	defer lock.Unlock()
	logger.Debug("restart Xray, force:", isForce)
	isManuallyStopped.Store(false)

	xrayConfig, err := s.GetXrayConfig()
	if err != nil {
		return err
	}

	if s.IsXrayRunning() {
		if !isForce && p.GetConfig().Equals(xrayConfig) && !isNeedXrayRestart.Load() {
			logger.Debug("It does not need to restart Xray")
			return nil
		}
		p.Stop()
	}

	p = xray.NewProcess(xrayConfig)
	result = ""
	err = p.Start()
	if err != nil {
		return err
	}

	return nil
}

// StopXray stops the running Xray process.
func (s *XrayService) StopXray() error {
	lock.Lock()
	defer lock.Unlock()
	isManuallyStopped.Store(true)
	logger.Debug("Attempting to stop Xray...")
	if s.IsXrayRunning() {
		return p.Stop()
	}
	return errors.New("xray is not running")
}

// SetToNeedRestart marks that Xray needs to be restarted.
func (s *XrayService) SetToNeedRestart() {
	isNeedXrayRestart.Store(true)
}

// IsNeedRestartAndSetFalse checks if restart is needed and resets the flag to false.
func (s *XrayService) IsNeedRestartAndSetFalse() bool {
	return isNeedXrayRestart.CompareAndSwap(true, false)
}

// DidXrayCrash checks if Xray crashed by verifying it's not running and wasn't manually stopped.
func (s *XrayService) DidXrayCrash() bool {
	return !s.IsXrayRunning() && !isManuallyStopped.Load()
}
