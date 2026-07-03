package modelsdev

type Catalog struct {
	Providers []Provider `json:"providers"`
}

type Provider struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	API    string   `json:"api,omitempty"`
	Env    []string `json:"env"`
	Doc    string   `json:"doc,omitempty"`
	Models []Model  `json:"models"`
}

type Model struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Family           string     `json:"family,omitempty"`
	Reasoning        bool       `json:"reasoning"`
	ToolCall         bool       `json:"tool_call"`
	StructuredOutput bool       `json:"structured_output"`
	Attachment       bool       `json:"attachment"`
	Temperature      bool       `json:"temperature"`
	Knowledge        string     `json:"knowledge,omitempty"`
	ReleaseDate      string     `json:"release_date,omitempty"`
	LastUpdated      string     `json:"last_updated,omitempty"`
	Modalities       Modalities `json:"modalities"`
	OpenWeights      bool       `json:"open_weights"`
	Cost             Cost       `json:"cost,omitempty"`
	Limit            Limit      `json:"limit,omitempty"`
	Status           string     `json:"status,omitempty"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type Cost struct {
	Input       float64 `json:"input,omitempty"`
	Output      float64 `json:"output,omitempty"`
	CacheRead   float64 `json:"cache_read,omitempty"`
	CacheWrite  float64 `json:"cache_write,omitempty"`
	InputAudio  float64 `json:"input_audio,omitempty"`
	OutputAudio float64 `json:"output_audio,omitempty"`
}

type Limit struct {
	Context int `json:"context,omitempty"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output,omitempty"`
}
