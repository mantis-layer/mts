package agentplugin

import (
	"fmt"
	"regexp"
)

var versionRe = regexp.MustCompile(`^\d+\.\d+(\.\d+)?([-+][0-9A-Za-z.-]+)?$`)

// ValidateManifest 校验 Manifest：name 非空、apiVersion 兼容、类型合法、版本格式合法。
func ValidateManifest(m PluginManifest) error {
	if m.Name == "" {
		return &ManifestError{Code: "missing_name", Message: "插件 name 不能为空", Manifest: m}
	}
	if m.APIVersion == "" {
		return &ManifestError{Code: "missing_api_version", Message: "插件 apiVersion 不能为空", Manifest: m}
	}
	if m.APIVersion != CurrentAPIVersion {
		return &ManifestError{
			Code:     "unsupported_api_version",
			Message:  fmt.Sprintf("apiVersion=%q 不受支持（当前 %q）", m.APIVersion, CurrentAPIVersion),
			Manifest: m,
		}
	}
	switch m.Type {
	case PluginTypeTool, PluginTypeModelProvider, PluginTypeEvaluator, PluginTypePattern:
	default:
		return &ManifestError{Code: "unknown_type", Message: fmt.Sprintf("未知插件类型 %q", m.Type), Manifest: m}
	}
	if err := validateVersion(m.Version); err != nil {
		return err
	}
	return nil
}

func validateVersion(v string) error {
	if v == "" {
		return &ManifestError{Code: "missing_version", Message: "插件 version 不能为空"}
	}
	if !versionRe.MatchString(v) {
		return &ManifestError{Code: "invalid_version", Message: fmt.Sprintf("非法版本格式 %q（期望 semver，如 1.0.0）", v)}
	}
	return nil
}
