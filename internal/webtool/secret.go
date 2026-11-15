package webtool

import (
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/sealer"
)

// SECURITY: the AAD binds a sealed search key to its org and project, so bytes copied to another project never open.
func searchKeyAAD(orgID, projectID uuid.UUID) []byte {
	return []byte("webtool:" + orgID.String() + ":" + projectID.String() + ":search_api_key")
}

// SECURITY: the AAD binds a sealed value to its org, project, host and header, so a swapped row never sends it elsewhere.
func secretAAD(orgID, projectID uuid.UUID, host, header string) []byte {
	return []byte("webtool:" + orgID.String() + ":" + projectID.String() + ":" + host + ":" + header + ":http_secret")
}

func sealSearchKey(sl sealer.Sealer, orgID, projectID uuid.UUID, key string) ([]byte, error) {
	return seal(sl, []byte(key), searchKeyAAD(orgID, projectID))
}

func openSearchKey(sl sealer.Sealer, orgID, projectID uuid.UUID, sealed []byte) (string, error) {
	return open(sl, sealed, searchKeyAAD(orgID, projectID))
}

func sealSecret(sl sealer.Sealer, orgID, projectID uuid.UUID, host, header, value string) ([]byte, error) {
	return seal(sl, []byte(value), secretAAD(orgID, projectID, host, header))
}

func openSecret(sl sealer.Sealer, orgID, projectID uuid.UUID, host, header string, sealed []byte) (string, error) {
	return open(sl, sealed, secretAAD(orgID, projectID, host, header))
}

func seal(sl sealer.Sealer, plain, aad []byte) ([]byte, error) {
	sealed, err := sl.Seal(plain, aad)
	if err != nil {
		return nil, secretsErr(fmt.Errorf("webtool.seal: %w", err))
	}
	return sealed, nil
}

func open(sl sealer.Sealer, sealed, aad []byte) (string, error) {
	plain, err := sl.Open(sealed, aad)
	if err != nil {
		return "", secretsErr(fmt.Errorf("webtool.open: %w", err))
	}
	return string(plain), nil
}

func secretsErr(err error) error {
	if sealer.IsUnavailableError(err) {
		return &SecretsUnavailableError{Cause: err}
	}
	return err
}
