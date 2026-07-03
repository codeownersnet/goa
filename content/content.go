package content

type Role string

const (
	RoleUser   Role = "user"
	RoleModel  Role = "model"
	RoleSystem Role = "system"
	RoleTool   Role = "tool"
)

type Content struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

func NewContent(role Role, parts ...Part) *Content {
	return &Content{Role: role, Parts: parts}
}

func NewTextContent(text string, role Role) *Content {
	return &Content{Role: role, Parts: []Part{NewTextPart(text)}}
}

type Part struct {
	Text             *TextPart          `json:"text,omitempty"`
	InlineData       *InlineDataPart    `json:"inline_data,omitempty"`
	FileData         *FileDataPart      `json:"file_data,omitempty"`
	FunctionCall     *FunctionCall      `json:"function_call,omitempty"`
	FunctionResponse *FunctionResponse  `json:"function_response,omitempty"`
	Thinking         *ThinkingPart      `json:"thinking,omitempty"`
	CodeExecution    *CodeExecutionPart `json:"code_execution,omitempty"`
}

func NewTextPart(text string) Part {
	return Part{Text: &TextPart{Text: text}}
}

func NewInlineDataPart(mimeType string, data []byte) Part {
	return Part{InlineData: &InlineDataPart{MIMEType: mimeType, Data: data}}
}

func NewFileDataPart(fileURI, mimeType string) Part {
	return Part{FileData: &FileDataPart{FileURI: fileURI, MIMEType: mimeType}}
}

func NewFunctionCallPart(id, name string, args map[string]any) Part {
	return Part{FunctionCall: &FunctionCall{ID: id, Name: name, Args: args}}
}

func NewFunctionResponsePart(id, name string, response map[string]any, isError bool) Part {
	return Part{FunctionResponse: &FunctionResponse{ID: id, Name: name, Response: response, IsError: isError}}
}

func NewThinkingPart(text string) Part {
	return Part{Thinking: &ThinkingPart{Text: text}}
}

func (p Part) Type() string {
	switch {
	case p.Text != nil:
		return "text"
	case p.InlineData != nil:
		return "inline_data"
	case p.FileData != nil:
		return "file_data"
	case p.FunctionCall != nil:
		return "function_call"
	case p.FunctionResponse != nil:
		return "function_response"
	case p.Thinking != nil:
		return "thinking"
	case p.CodeExecution != nil:
		return "code_execution"
	default:
		return "unknown"
	}
}

type TextPart struct {
	Text string `json:"text"`
}

type InlineDataPart struct {
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

type FileDataPart struct {
	FileURI  string `json:"file_uri"`
	MIMEType string `json:"mime_type"`
}

type CodeExecutionPart struct {
	Code   string `json:"code"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type ThinkingPart struct {
	Text      string `json:"text"`
	Signature []byte `json:"signature,omitempty"`
}
