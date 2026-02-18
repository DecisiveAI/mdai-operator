package v1

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This test protects webhook resource names against typos by verifying that every
// resource referenced in webhook markers/manifests matches a CRD plural name.
func TestWebhookResourceNamesMatchCRDPlurals(t *testing.T) {
	t.Parallel()

	plurals := loadCRDPlurals(t)
	if len(plurals) == 0 {
		t.Fatal("no CRD plurals found; CRD fixtures may be missing")
	}

	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	targets := []string{
		filepath.Join(repoRoot, "internal", "webhook", "v1", "mdaiingress_webhook.go"),
		filepath.Join(repoRoot, "internal", "webhook", "v1", "mdaihub_webhook.go"),
		filepath.Join(repoRoot, "internal", "webhook", "v1", "mdaicollector_webhook.go"),
		filepath.Join(repoRoot, "internal", "webhook", "v1", "mdaiobserver_webhook.go"),
		filepath.Join(repoRoot, "internal", "webhook", "v1", "mdaireplay_webhook.go"),
		filepath.Join(repoRoot, "config", "webhook", "manifests.yaml"),
		filepath.Join(repoRoot, "deployment", "templates", "validating-webhook-configuration.yaml"),
		filepath.Join(repoRoot, "deployment", "templates", "no-cm-secrets-webhooks.yaml"),
	}

	for _, path := range targets {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}

		for _, resource := range extractWebhookResources(string(content)) {
			if !plurals[resource] {
				t.Fatalf("file %s references unknown webhook resource %q; did you typo the CRD plural?", path, resource)
			}
		}
	}
}

func loadCRDPlurals(t *testing.T) map[string]bool {
	t.Helper()

	crdDir := filepath.Clean(filepath.Join("..", "..", "..", "config", "crd", "bases"))
	files, err := os.ReadDir(crdDir)
	if err != nil {
		t.Fatalf("failed to read CRD directory %s: %v", crdDir, err)
	}

	result := make(map[string]bool)
	rePlural := regexp.MustCompile(`(?m)^\s*plural:\s*([A-Za-z0-9]+)\s*$`)

	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(crdDir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read CRD file %s: %v", entry.Name(), err)
		}

		for _, match := range rePlural.FindAllStringSubmatch(string(content), -1) {
			if len(match) > 1 {
				result[match[1]] = true
			}
		}
	}

	return result
}

func extractWebhookResources(content string) []string {
	reMarker := regexp.MustCompile(`resources=([a-z0-9]+)`)
	found := make(map[string]bool)

	for _, match := range reMarker.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			found[match[1]] = true
		}
	}

	for _, res := range collectYAMLResources(content) {
		found[res] = true
	}

	var out []string
	for res := range found {
		out = append(out, res)
	}
	return out
}

func collectYAMLResources(content string) []string {
	lines := strings.Split(content, "\n")
	var resources []string
	inResources := false
	var indent string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "resources:") {
			inResources = true
			indent = leadingSpaces(line)
			continue
		}

		if inResources {
			if strings.HasPrefix(line, indent+"  -") || strings.HasPrefix(line, indent+"-") {
				token := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, indent+"-"), indent+"  -"))
				if token != "" {
					resources = append(resources, token)
				}
				continue
			}

			if leadingSpaces(line) != indent || trimmed == "" {
				inResources = false
			}
		}
	}

	return resources
}

func leadingSpaces(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}
