// Package conformance runs against the *published* sandbox image and chart.
// Verifies cosign signatures and that the embedded bundle digest matches
// what the binary self-reports. Enabled by build tag "conformance".
//
//go:build conformance

package conformance
