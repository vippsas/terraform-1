package secureazurerm

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform/backend/remote-state/secureazurerm/remote"
	"github.com/hashicorp/terraform/backend/remote-state/secureazurerm/remote/account/blob"
	"github.com/hashicorp/terraform/backend/remote-state/secureazurerm/remote/keyvault"
	"github.com/hashicorp/terraform/config/module"
	"github.com/hashicorp/terraform/state"
	"github.com/hashicorp/terraform/terraform"
)

// States returns a list of the names of all remote states stored in separate unique blob.
// They are all named after the workspace.
// Basically, remote state = workspace = blob.
func (b *Backend) States() ([]string, error) {
	// Get the blobs of the container.
	blobs, err := b.container.List()
	if err != nil {
		return nil, fmt.Errorf("error listing blobs: %s", err)
	}

	// List workspaces (which is equivalent to blobs) in the container.
	workspaces := []string{}
	for _, blob := range blobs {
		workspaces = append(workspaces, blob.Name)
	}

	return workspaces, nil
}

// DeleteState deletes remote state.
func (b *Backend) DeleteState(name string) error {
	// Setup state blob.
	blob, err := blob.Setup(b.container, name) // blob name = workspace name.
	if err != nil {
		return fmt.Errorf("error setting up state blob: %s", err)
	}

	// Setup state key vault
	keyVault, err := b.setupKeyVault(blob, name)
	if err != nil {
		return fmt.Errorf("error setting up state key vault: %s", err)
	}
	// and delete it!

	keyVault.Delete(context.Background())
	if err := blob.Delete(); err != nil {
		return fmt.Errorf("error deleting state %s: %s", name, err)
	}

	return nil
}

// State returns the state specified by workspace.
func (b *Backend) State(workspaceName string) (state.State, error) {
	// Setup blob.
	blob, err := blob.Setup(b.container, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("error setting up state blob: %s", err)
	}

	// Setup key vault.
	keyVault, err := b.setupKeyVault(blob, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("error setting up state key vault: %s", err)
	}

	providers, err := GetProviders(b.ContextOpts)
	if err != nil {
		return nil, fmt.Errorf("error retrieving providers: %s", err)
	}
	return &remote.State{Blob: blob, KeyVault: keyVault, ResourceProviders: providers, Props: &b.props}, nil
}

// setupKeyVault setups the state key vault.
func (b *Backend) setupKeyVault(blob *blob.Blob, workspaceName string) (*keyvault.KeyVault, error) {
	keyVault, err := keyvault.Setup(context.Background(), blob, &b.props, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("error setting up key vault: %s", err)
	}
	return keyVault, nil
}

// GetProviders returns all the resource providers for the given configuration.
func GetProviders(opts *terraform.ContextOpts) ([]terraform.ResourceProvider, error) {
	mod := opts.Module
	if mod == nil {
		pwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting pwd: %s", err)
		}
		mod, err = module.NewTreeModule("", pwd)
		if err != nil {
			return nil, err
		}
	}
	reqd := terraform.ModuleTreeDependencies(mod, nil).AllPluginRequirements()
	if opts.ProviderSHA256s != nil && !opts.SkipProviderVerify {
		reqd.LockExecutables(opts.ProviderSHA256s)
	}
	providerFactories, errs := opts.ProviderResolver.ResolveProviders(reqd)
	if errs != nil {
		return nil, &terraform.ResourceProviderError{
			Errors: errs,
		}
	}
	var providers []terraform.ResourceProvider
	for _, f := range providerFactories {
		provider, err := f()
		if err != nil {
			return nil, fmt.Errorf("error retrieving provider: %s", err)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}
