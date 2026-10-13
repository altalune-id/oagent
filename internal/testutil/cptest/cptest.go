// Package cptest holds controlplane test helpers that would cycle inside fakes.
package cptest

import (
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/testutil/fakes"
)

// OperatorOptions makes a controlplane server accept fakes.OperatorToken as the system operator.
func OperatorOptions() controlplane.Options {
	return controlplane.Options{Authn: authn.InterceptorOptions{
		Chain:       authn.Chain{fakes.OperatorAuthn{}},
		KeyPrefix:   "oagent_",
		Permissions: permission.Table(),
	}}
}

// OperatorCredential is the client side of OperatorOptions.
func OperatorCredential() apiclient.Credential {
	return apiclient.Credential{Token: fakes.OperatorToken}
}
