package onebot11

import "JuanNiang-Neo/internal/provider"

func (r *OneBot11Provider) ID() string {
	return r.ProviderID
}

func (r *OneBot11Provider) Name() string {
	return r.ProviderName
}

func (r *OneBot11Provider) Platform() provider.Platform {
	return r.ProviderPlatform
}
