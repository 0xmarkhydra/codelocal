package plugins

const SchemaVersion = 1

type Scope string
type Distribution string
type ComponentKind string
type AppTransport string
type AuthKind string
type Capability string

const (
	ScopeSystem    Scope = "system"
	ScopePersonal  Scope = "personal"
	ScopeCommunity Scope = "community"
	ScopeInternal  Scope = "internal"
)

const (
	DistributionPrivate  Distribution = "private"
	DistributionInternal Distribution = "internal"
	DistributionPublic   Distribution = "public"
)

const (
	ComponentApp         ComponentKind = "app"
	ComponentSkill       ComponentKind = "skill"
	ComponentAppTemplate ComponentKind = "app_template"
)

const (
	TransportMCPHTTP       AppTransport = "mcp_http"
	TransportMCPStdioLocal AppTransport = "mcp_stdio_local"
)

const (
	AuthNone            AuthKind = "none"
	AuthOAuth2          AuthKind = "oauth2"
	AuthEnvReference    AuthKind = "env_reference"
	AuthHeaderReference AuthKind = "header_reference"
)

const (
	CapabilityProjectRead      Capability = "project_read"
	CapabilityProjectWrite     Capability = "project_write"
	CapabilityLocalShell       Capability = "local_shell"
	CapabilityLocalBrowser     Capability = "local_browser"
	CapabilityLocalCredentials Capability = "local_credentials"
	CapabilityExternalRead     Capability = "external_read"
	CapabilityExternalWrite    Capability = "external_write"
	CapabilityExternalDelete   Capability = "external_delete"
	CapabilityNetwork          Capability = "network"
	CapabilityComputerControl  Capability = "computer_control"
	CapabilityBackgroundTask   Capability = "background_task"
)

type Publisher struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Verified bool   `json:"verified,omitempty"`
}

type Icon struct {
	AssetID string `json:"assetId,omitempty"`
	URL     string `json:"url,omitempty"`
}

type AuthDefinition struct {
	Kind AuthKind `json:"kind"`
}

type AppDefinition struct {
	Transport     AppTransport   `json:"transport"`
	Endpoint      string         `json:"endpoint,omitempty"`
	Command       string         `json:"command,omitempty"`
	Args          []string       `json:"args,omitempty"`
	Auth          AuthDefinition `json:"auth"`
	Capabilities  []Capability   `json:"capabilities,omitempty"`
	ToolNamespace string         `json:"toolNamespace,omitempty"`
}

type SkillReference struct {
	SkillID string `json:"skillId"`
	Version string `json:"version"`
}

type AppTemplateField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Description string `json:"description,omitempty"`
}

type AppTemplateDefinition struct {
	Transport AppTransport       `json:"transport"`
	Auth      AuthDefinition     `json:"auth"`
	Fields    []AppTemplateField `json:"fields,omitempty"`
}

type Component struct {
	Kind        ComponentKind          `json:"kind"`
	ID          string                 `json:"id"`
	App         *AppDefinition         `json:"app,omitempty"`
	Skill       *SkillReference        `json:"skill,omitempty"`
	AppTemplate *AppTemplateDefinition `json:"appTemplate,omitempty"`
}

type Manifest struct {
	SchemaVersion    int          `json:"schemaVersion"`
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	Publisher        Publisher    `json:"publisher"`
	Description      string       `json:"description,omitempty"`
	Icon             *Icon        `json:"icon,omitempty"`
	Categories       []string     `json:"categories,omitempty"`
	Components       []Component  `json:"components"`
	Scope            Scope        `json:"scope"`
	Distribution     Distribution `json:"distribution"`
	PrivacyPolicyURL string       `json:"privacyPolicyUrl,omitempty"`
	TermsURL         string       `json:"termsUrl,omitempty"`
}
