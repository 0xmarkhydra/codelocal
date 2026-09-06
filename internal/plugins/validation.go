package plugins

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	canonicalIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	semverRE      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	fieldKeyRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
)

func ValidateManifest(manifest Manifest) error {
	if err := validateManifestIdentity(manifest); err != nil {
		return err
	}
	if err := validateManifestMetadata(manifest); err != nil {
		return err
	}
	return validateManifestComponents(manifest)
}

func validateManifestIdentity(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported plugin schemaVersion %d", manifest.SchemaVersion)
	}
	if err := validateID("plugin id", manifest.ID); err != nil {
		return err
	}
	if err := validateText("plugin name", manifest.Name, true); err != nil {
		return err
	}
	if !semverRE.MatchString(manifest.Version) {
		return fmt.Errorf("plugin version must be semantic version, got %q", manifest.Version)
	}
	if err := validateID("publisher id", manifest.Publisher.ID); err != nil {
		return err
	}
	return validateText("publisher name", manifest.Publisher.Name, true)
}

func validateManifestMetadata(manifest Manifest) error {
	if err := validateScopeDistribution(manifest.Scope, manifest.Distribution); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("privacyPolicyUrl", manifest.PrivacyPolicyURL, manifest.Distribution == DistributionPublic); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("termsUrl", manifest.TermsURL, manifest.Distribution == DistributionPublic); err != nil {
		return err
	}
	return validateIcon(manifest.Icon, manifest.Distribution)
}

func validateIcon(icon *Icon, distribution Distribution) error {
	if icon == nil {
		return nil
	}
	if strings.TrimSpace(icon.AssetID) == "" && strings.TrimSpace(icon.URL) == "" {
		return fmt.Errorf("plugin icon must define assetId or url")
	}
	if icon.AssetID != "" {
		if err := validateText("icon assetId", icon.AssetID, true); err != nil {
			return err
		}
	}
	if icon.URL != "" {
		return validateOptionalHTTPURL("icon url", icon.URL, distribution == DistributionPublic)
	}
	return nil
}

func validateManifestComponents(manifest Manifest) error {
	if len(manifest.Components) == 0 {
		return fmt.Errorf("plugin must contain at least one component")
	}
	seenComponents := make(map[string]struct{}, len(manifest.Components))
	requiresPrivacy := false
	for i, component := range manifest.Components {
		if err := validateID(fmt.Sprintf("component[%d] id", i), component.ID); err != nil {
			return err
		}
		if _, exists := seenComponents[component.ID]; exists {
			return fmt.Errorf("duplicate component id %q", component.ID)
		}
		seenComponents[component.ID] = struct{}{}
		componentNeedsPrivacy, err := validateComponent(manifest, component)
		if err != nil {
			return fmt.Errorf("component %q: %w", component.ID, err)
		}
		requiresPrivacy = requiresPrivacy || componentNeedsPrivacy
	}
	if manifest.Distribution == DistributionPublic && requiresPrivacy && strings.TrimSpace(manifest.PrivacyPolicyURL) == "" {
		return fmt.Errorf("public plugin with external data capabilities requires privacyPolicyUrl")
	}
	return nil
}

func validateComponent(manifest Manifest, component Component) (bool, error) {
	switch component.Kind {
	case ComponentApp:
		if component.App == nil || component.Skill != nil || component.AppTemplate != nil {
			return false, fmt.Errorf("app component must define only app")
		}
		return validateApp(manifest, *component.App)
	case ComponentSkill:
		if component.Skill == nil || component.App != nil || component.AppTemplate != nil {
			return false, fmt.Errorf("skill component must define only skill")
		}
		return false, validateSkillReference(*component.Skill)
	case ComponentAppTemplate:
		if component.AppTemplate == nil || component.App != nil || component.Skill != nil {
			return false, fmt.Errorf("app_template component must define only appTemplate")
		}
		return validateAppTemplate(manifest, *component.AppTemplate)
	default:
		return false, fmt.Errorf("unsupported component kind %q", component.Kind)
	}
}

func validateApp(manifest Manifest, app AppDefinition) (bool, error) {
	if err := validateAuth(app.Auth); err != nil {
		return false, err
	}
	needsPrivacy, err := validateCapabilities(app.Capabilities)
	if err != nil {
		return false, err
	}
	if app.ToolNamespace != "" {
		if err := validateID("toolNamespace", app.ToolNamespace); err != nil {
			return false, err
		}
	}
	if err := validateExecutionTargets(app.Execution); err != nil {
		return false, err
	}
	switch app.Transport {
	case TransportMCPHTTP:
		if strings.TrimSpace(app.Command) != "" || len(app.Args) != 0 {
			return false, fmt.Errorf("mcp_http app cannot define command or args")
		}
		if err := validateMCPEndpoint(app.Endpoint, manifest.Distribution); err != nil {
			return false, err
		}
	case TransportMCPStdioLocal:
		if manifest.Distribution == DistributionPublic {
			return false, fmt.Errorf("public plugin cannot use mcp_stdio_local")
		}
		if manifest.Scope != ScopePersonal && manifest.Scope != ScopeInternal {
			return false, fmt.Errorf("mcp_stdio_local requires personal or internal scope")
		}
		if strings.TrimSpace(app.Command) == "" {
			return false, fmt.Errorf("mcp_stdio_local app requires command")
		}
		if app.Command != strings.TrimSpace(app.Command) || strings.ContainsAny(app.Command, "\x00\r\n") {
			return false, fmt.Errorf("mcp_stdio_local command must be canonical")
		}
		if strings.TrimSpace(app.Endpoint) != "" {
			return false, fmt.Errorf("mcp_stdio_local app cannot define endpoint")
		}
		for i, arg := range app.Args {
			if strings.ContainsAny(arg, "\x00\r\n") {
				return false, fmt.Errorf("arg %d contains control characters", i)
			}
		}
	default:
		return false, fmt.Errorf("unsupported app transport %q", app.Transport)
	}
	return needsPrivacy, nil
}

func validateAppTemplate(manifest Manifest, template AppTemplateDefinition) (bool, error) {
	if err := validateAuth(template.Auth); err != nil {
		return false, err
	}
	needsPrivacy, err := validateCapabilities(template.Capabilities)
	if err != nil {
		return false, err
	}
	if err := validateExecutionTargets(template.Execution); err != nil {
		return false, err
	}
	switch template.Transport {
	case TransportMCPHTTP:
	case TransportMCPStdioLocal:
		if manifest.Distribution == DistributionPublic {
			return false, fmt.Errorf("public plugin cannot template mcp_stdio_local")
		}
		if manifest.Scope != ScopePersonal && manifest.Scope != ScopeInternal {
			return false, fmt.Errorf("mcp_stdio_local template requires personal or internal scope")
		}
	default:
		return false, fmt.Errorf("unsupported app template transport %q", template.Transport)
	}
	seen := make(map[string]struct{}, len(template.Fields))
	for i, field := range template.Fields {
		if !fieldKeyRE.MatchString(field.Key) {
			return false, fmt.Errorf("template field %d has invalid key %q", i, field.Key)
		}
		if _, ok := seen[field.Key]; ok {
			return false, fmt.Errorf("duplicate template field key %q", field.Key)
		}
		seen[field.Key] = struct{}{}
		if err := validateText(fmt.Sprintf("template field %q label", field.Key), field.Label, true); err != nil {
			return false, err
		}
	}
	return needsPrivacy || template.Transport == TransportMCPHTTP, nil
}

func validateExecutionTargets(targets []ExecutionTarget) error {
	seen := make(map[ExecutionTarget]struct{}, len(targets))
	for _, target := range targets {
		switch target {
		case ExecutionLocal, ExecutionCloud:
		default:
			return fmt.Errorf("unsupported execution target %q", target)
		}
		if _, exists := seen[target]; exists {
			return fmt.Errorf("duplicate execution target %q", target)
		}
		seen[target] = struct{}{}
	}
	return nil
}

func validateSkillReference(skill SkillReference) error {
	if err := validateID("skillId", skill.SkillID); err != nil {
		return err
	}
	if !semverRE.MatchString(skill.Version) {
		return fmt.Errorf("skill version must be semantic version, got %q", skill.Version)
	}
	return nil
}

func validateCapabilities(capabilities []Capability) (bool, error) {
	seen := make(map[Capability]struct{}, len(capabilities))
	needsPrivacy := false
	for _, capability := range capabilities {
		if !knownCapability(capability) {
			return false, fmt.Errorf("unsupported capability %q", capability)
		}
		if _, exists := seen[capability]; exists {
			return false, fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		switch capability {
		case CapabilityExternalRead, CapabilityExternalWrite, CapabilityExternalDelete, CapabilityNetwork:
			needsPrivacy = true
		}
	}
	return needsPrivacy, nil
}

func knownCapability(capability Capability) bool {
	switch capability {
	case CapabilityProjectRead, CapabilityProjectWrite, CapabilityLocalShell,
		CapabilityLocalBrowser, CapabilityLocalCredentials, CapabilityExternalRead,
		CapabilityExternalWrite, CapabilityExternalDelete, CapabilityNetwork,
		CapabilityComputerControl, CapabilityBackgroundTask:
		return true
	default:
		return false
	}
}

func validateAuth(auth AuthDefinition) error {
	switch auth.Kind {
	case AuthNone, AuthOAuth2, AuthEnvReference, AuthHeaderReference:
		return nil
	default:
		return fmt.Errorf("unsupported auth kind %q", auth.Kind)
	}
}

func validateScopeDistribution(scope Scope, distribution Distribution) error {
	switch scope {
	case ScopeSystem, ScopePersonal, ScopeCommunity, ScopeInternal:
	default:
		return fmt.Errorf("unsupported plugin scope %q", scope)
	}
	switch distribution {
	case DistributionPrivate, DistributionInternal, DistributionPublic:
	default:
		return fmt.Errorf("unsupported plugin distribution %q", distribution)
	}
	switch scope {
	case ScopePersonal:
		if distribution != DistributionPrivate {
			return fmt.Errorf("personal plugin must use private distribution")
		}
	case ScopeCommunity:
		if distribution != DistributionPublic {
			return fmt.Errorf("community plugin must use public distribution")
		}
	case ScopeInternal:
		if distribution != DistributionInternal {
			return fmt.Errorf("internal plugin must use internal distribution")
		}
	case ScopeSystem:
		if distribution == DistributionPrivate {
			return fmt.Errorf("system plugin cannot use private distribution")
		}
	}
	return nil
}

func validateMCPEndpoint(raw string, distribution Distribution) error {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("mcp_http app requires valid absolute endpoint")
	}
	if raw != trimmed {
		return fmt.Errorf("mcp endpoint must be canonical")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("mcp endpoint must use http or https")
	}
	if scheme == "http" && (distribution == DistributionPublic || !isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("insecure mcp http endpoint is allowed only for non-public loopback development")
	}
	return nil
}

func validateOptionalHTTPURL(field, raw string, public bool) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if raw != strings.TrimSpace(raw) {
		return fmt.Errorf("%s must be canonical", field)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("%s must be an absolute URL", field)
	}
	if public && strings.ToLower(u.Scheme) != "https" {
		return fmt.Errorf("%s must use https for public plugins", field)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", field)
	}
	return nil
}

func validateID(field, value string) error {
	if !canonicalIDRE.MatchString(value) {
		return fmt.Errorf("%s must match %s", field, canonicalIDRE.String())
	}
	return nil
}

func validateText(field, value string, required bool) error {
	trimmed := strings.TrimSpace(value)
	if required && trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != trimmed || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must be canonical single-line text", field)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" || host == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}
