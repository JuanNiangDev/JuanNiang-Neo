package tool

func (t *ToolGroup) AddTool(conf *ToolConfig) error {}

func (t *ToolGroup) DelTool(toolID string) error {}

func (t *ToolGroup) GetTool(toolID string) (Tool, error) {}

func (t *ToolGroup) ListTools() ([]Tool, error) {}

func (t *ToolGroup) EditToolConfig(toolID string, conf *ToolConfig) error {}
