package onebot11

import "JuanNiang-Neo/internal/provider"

type BasicInfo struct {
	ProviderID   string
	ProviderName string
	ListenAddr   string
	ListenPort   int
	Token        string
}

type OneBot11Provider struct {
	ProviderID       string
	ProviderName     string
	ListenAddr       string
	ListenPort       int
	Token            string
	ProviderPlatform provider.Platform
}

func NewOneBot11Provider(conf BasicInfo) *OneBot11Provider {
	return &OneBot11Provider{
		ProviderID:       conf.ProviderID,
		ProviderName:     conf.ProviderName,
		ListenAddr:       conf.ListenAddr,
		ListenPort:       conf.ListenPort,
		Token:            conf.Token,
		ProviderPlatform: provider.PlatformOneBot11,
	}
}
