//go:build conformance

package conformance

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBinarySelfReportsBundleDigest(t *testing.T) {
	bin := os.Getenv("CONFORMANCE_BINARY")
	if bin == "" {
		t.Skip("CONFORMANCE_BINARY env var not set; skipping")
	}
	out, err := exec.Command(bin, "--print-bundle-digest").Output()
	if err != nil {
		t.Fatalf("--print-bundle-digest: %v", err)
	}
	digest := strings.TrimSpace(string(out))
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("digest format: %q", digest)
	}
	expected := os.Getenv("EXPECTED_BUNDLE_DIGEST")
	if expected != "" && digest != expected {
		t.Errorf("digest = %q, expected %q", digest, expected)
	}
}

func TestImageCosignSignature(t *testing.T) {
	imageRef := os.Getenv("CONFORMANCE_IMAGE_REF")
	identity := os.Getenv("CONFORMANCE_COSIGN_IDENTITY")
	issuer := os.Getenv("CONFORMANCE_COSIGN_OIDC_ISSUER")
	if imageRef == "" || identity == "" {
		t.Skip("CONFORMANCE_IMAGE_REF / CONFORMANCE_COSIGN_IDENTITY not set; skipping")
	}
	if issuer == "" {
		issuer = "https://token.actions.githubusercontent.com"
	}
	out, err := exec.Command("cosign", "verify",
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", issuer,
		imageRef,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("cosign verify failed: %v\n%s", err, out)
	}
}

func TestChartCosignSignature(t *testing.T) {
	chartRef := os.Getenv("CONFORMANCE_CHART_REF")
	identity := os.Getenv("CONFORMANCE_COSIGN_IDENTITY")
	issuer := os.Getenv("CONFORMANCE_COSIGN_OIDC_ISSUER")
	if chartRef == "" || identity == "" {
		t.Skip("CONFORMANCE_CHART_REF / CONFORMANCE_COSIGN_IDENTITY not set; skipping")
	}
	if issuer == "" {
		issuer = "https://token.actions.githubusercontent.com"
	}
	out, err := exec.Command("cosign", "verify",
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", issuer,
		chartRef,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("chart cosign verify failed: %v\n%s", err, out)
	}
}
