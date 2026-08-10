package clientupdate

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Level string

const (
	Recommended Level = "recommended"
	Required    Level = "required"
)

type Manifest struct {
	LatestVersion  string `json:"latestVersion,omitempty"`
	MinimumVersion string `json:"minimumVersion,omitempty"`
	Channel        string `json:"channel"`
	UpdateCommand  string `json:"updateCommand"`
	RestartCommand string `json:"restartCommand"`
	Message        string `json:"message"`
}

type Notice struct {
	Key              string `json:"key"`
	Level            Level  `json:"level"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	LatestVersion    string `json:"latestVersion"`
	MinimumVersion   string `json:"minimumVersion,omitempty"`
	Channel          string `json:"channel"`
	UpdateCommand    string `json:"updateCommand"`
	RestartCommand   string `json:"restartCommand"`
	Message          string `json:"message"`
}

type semver struct {
	major, minor, patch int
	prerelease          []string
}

var semverRE = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var channelRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,31}$`)
var numericRE = regexp.MustCompile(`^\d+$`)

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1 && (value[0] == 'v' || value[0] == 'V') && value[1] >= '0' && value[1] <= '9' {
		value = value[1:]
	}
	return value
}

func parse(value string) (semver, bool) {
	value = normalizeVersion(value)
	m := semverRE.FindStringSubmatch(value)
	if m == nil {
		return semver{}, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	patch, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return semver{}, false
	}
	var pre []string
	if m[4] != "" {
		pre = strings.Split(m[4], ".")
	}
	return semver{major: major, minor: minor, patch: patch, prerelease: pre}, true
}

func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	limit := len(a)
	if len(b) > limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		if a[i] == b[i] {
			continue
		}
		aNum := numericRE.MatchString(a[i])
		bNum := numericRE.MatchString(b[i])
		if aNum && bNum {
			av, _ := strconv.ParseUint(a[i], 10, 64)
			bv, _ := strconv.ParseUint(b[i], 10, 64)
			if av < bv {
				return -1
			}
			return 1
		}
		if aNum != bNum {
			if aNum {
				return -1
			}
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
		return 1
	}
	return 0
}

func Compare(left, right string) (int, bool) {
	a, ok := parse(left)
	if !ok {
		return 0, false
	}
	b, ok := parse(right)
	if !ok {
		return 0, false
	}
	if a.major != b.major {
		if a.major < b.major {
			return -1, true
		}
		return 1, true
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1, true
		}
		return 1, true
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1, true
		}
		return 1, true
	}
	return comparePrerelease(a.prerelease, b.prerelease), true
}

func ManifestFromEnv() Manifest {
	channel := strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_CHANNEL"))
	if !channelRE.MatchString(channel) {
		channel = "beta"
	}
	defaultUpdate := "npm i -g codelocal@" + channel
	update := strings.TrimSpace(os.Getenv("CODELOCAL_UPDATE_COMMAND"))
	if update == "" {
		update = defaultUpdate
	}
	restart := strings.TrimSpace(os.Getenv("CODELOCAL_RESTART_COMMAND"))
	if restart == "" {
		restart = "codelocal"
	}
	message := strings.TrimSpace(os.Getenv("CODELOCAL_UPDATE_MESSAGE"))
	if message == "" {
		message = "CodeLocal has a new version available. Please tell the user to update."
	}
	return Manifest{
		LatestVersion:  normalizeVersion(os.Getenv("CODELOCAL_LATEST_CLIENT_VERSION")),
		MinimumVersion: normalizeVersion(os.Getenv("CODELOCAL_MIN_CLIENT_VERSION")),
		Channel:        channel,
		UpdateCommand:  update,
		RestartCommand: restart,
		Message:        message,
	}
}

func Evaluate(installedVersion string, manifest Manifest) *Notice {
	latest := normalizeVersion(manifest.LatestVersion)
	if _, ok := parse(latest); !ok {
		return nil
	}
	installed := normalizeVersion(installedVersion)
	if installed != "" {
		if cmp, ok := Compare(installed, latest); ok && cmp >= 0 {
			return nil
		}
	}
	level := Recommended
	minimum := normalizeVersion(manifest.MinimumVersion)
	if installed != "" && minimum != "" {
		if _, ok := parse(minimum); ok {
			if cmp, comparable := Compare(installed, minimum); comparable && cmp < 0 {
				level = Required
			}
		}
	}
	return &Notice{
		Key:              "client-update:" + latest,
		Level:            level,
		InstalledVersion: installed,
		LatestVersion:    latest,
		MinimumVersion:   minimum,
		Channel:          manifest.Channel,
		UpdateCommand:    manifest.UpdateCommand,
		RestartCommand:   manifest.RestartCommand,
		Message:          manifest.Message,
	}
}

func Render(notice Notice) string {
	installed := notice.InstalledVersion
	if installed == "" {
		installed = "unknown (legacy client)"
	}
	urgency := "Ask the user to update, while still answering their current request normally."
	if notice.Level == Required {
		urgency = "This client is below the configured minimum version. Ask the user to update now."
	}
	lines := []string{
		"[CODELOCAL_UPDATE_NOTICE]",
		"IMPORTANT FOR CHATGPT: Surface this CodeLocal update notice to the user in your final response.",
		notice.Message,
		fmt.Sprintf("Update level: %s", notice.Level),
		"Installed: " + installed,
		"Latest: " + notice.LatestVersion,
	}
	if notice.MinimumVersion != "" {
		lines = append(lines, "Minimum supported client: "+notice.MinimumVersion)
	}
	lines = append(lines,
		"Update command: "+notice.UpdateCommand,
		"Then run: "+notice.RestartCommand,
		"Pairing and existing workspace grants do not need to be recreated after a normal update.",
		urgency,
		"Do not repeat this notice again in the same ChatGPT MCP session for this release.",
		"[/CODELOCAL_UPDATE_NOTICE]",
	)
	return strings.Join(lines, "\n")
}
