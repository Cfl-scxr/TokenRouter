package service

import (
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/qoder"
)

// qoderSiteForAccount 严格读取账号站点；旧账号缺失字段时默认国际站。
func qoderSiteForAccount(account *Account) (qoder.Site, error) {
	if account == nil {
		return qoder.SiteGlobal, fmt.Errorf("qoder: account is nil")
	}
	return qoder.ParseSite(account.GetCredential("site"))
}

// qoderProfileForAccount 返回账号站点对应的集中式协议 profile。
func qoderProfileForAccount(account *Account) (qoder.Profile, error) {
	site, err := qoderSiteForAccount(account)
	if err != nil {
		return qoder.Profile{}, err
	}
	return qoder.ProfileForSite(site)
}

// qoderRefreshModeForAccount 严格读取刷新模式；旧账号默认旧式 COSY。
func qoderRefreshModeForAccount(account *Account) (string, error) {
	if account == nil {
		return "", fmt.Errorf("qoder: account is nil")
	}
	return qoder.ParseRefreshMode(account.GetCredential("refresh_mode"))
}

// ensureQoderMachineCredentials 为新建 PAT 账号生成一次并持久化稳定机器身份。
func ensureQoderMachineCredentials(account *Account) {
	if account == nil || strings.TrimSpace(account.GetCredential("pat")) == "" {
		return
	}
	if account.Credentials == nil {
		account.Credentials = make(map[string]any)
	}
	machine := qoder.NewMachine()
	if strings.TrimSpace(account.GetCredential("machine_id")) == "" {
		account.Credentials["machine_id"] = machine.MachineID
	}
	if strings.TrimSpace(account.GetCredential("machine_token")) == "" {
		account.Credentials["machine_token"] = machine.MachineToken
	}
	if strings.TrimSpace(account.GetCredential("machine_type")) == "" {
		account.Credentials["machine_type"] = machine.MachineType
	}
}

// qoderMachineForAccount 读取持久化机器身份，并对旧账号使用兼容回退值。
func qoderMachineForAccount(account *Account) *qoder.MachineIdentity {
	if account == nil {
		return qoder.NewMachine()
	}
	machineID := strings.TrimSpace(account.GetCredential("machine_id"))
	if machineID == "" {
		machineID = qoder.RandomHex(36)
	}
	return &qoder.MachineIdentity{
		MachineID:    machineID,
		MachineToken: firstNonEmptyQoder(account.GetCredential("machine_token"), machineID),
		MachineType:  firstNonEmptyQoder(account.GetCredential("machine_type"), "5"),
	}
}

// qoderStreamClientForAccount 保留测试注入客户端，生产客户端则按账号站点重建端点和版本。
func qoderStreamClientForAccount(configured qoderStreamClient, account *Account) (qoderStreamClient, error) {
	if configured != nil {
		if _, productionClient := configured.(*qoder.Client); !productionClient {
			return configured, nil
		}
	}
	profile, err := qoderProfileForAccount(account)
	if err != nil {
		return nil, err
	}
	return qoder.NewClientForProfile(profile), nil
}
