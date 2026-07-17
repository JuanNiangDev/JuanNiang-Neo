package provider

func (p *ProviderGroup) AddProvider(conf *ProviderConfig) error {}

func (p *ProviderGroup) DelProvider(providerID string) error {}

func (p *ProviderGroup) GetProvider(providerID string) (Provider, error) {}

func (p *ProviderGroup) ListProvider() ([]Provider, error) {}

func (p *ProviderGroup) EditProviderConfig(providerID string, conf *ProviderConfig) error {}
