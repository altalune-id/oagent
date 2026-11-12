//go:build integration

// Package s3test gives integration tests a real S3 bucket: TEST_S3_* when set, otherwise one Garage testcontainer per test binary.
package s3test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"altalune.id/oagent/nanoid"
)

const (
	envEndpoint = "TEST_S3_ENDPOINT"
	envRegion   = "TEST_S3_REGION"
	envBucket   = "TEST_S3_BUCKET"
	envKeyID    = "TEST_S3_ACCESS_KEY_ID"
	envSecret   = "TEST_S3_SECRET_ACCESS_KEY"
	envCI       = "CI"
	image       = "dxflrs/garage:v2.4.1"
	labelOwner  = "id.altalune.oagent.s3test"
	staleAfter  = 30 * time.Minute
	prefixLen   = 12

	region     = "garage"
	bucket     = "oagent-test"
	keyID      = "GK0000000000000000000000a1"
	keySecret  = "00000000000000000000000000000000000000000000000000000000000000a1"
	adminToken = "oagent-test-garage-admin-token" //nolint:gosec // G101: a fixed token for a throwaway test container.
	corsOrigin = "http://localhost:5150"
	garageToml = `metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "0000000000000000000000000000000000000000000000000000000000000000"

[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.localhost"

[admin]
api_bind_addr = "[::]:3903"
admin_token = "` + adminToken + `"
`
)

//nolint:gochecknoglobals // one server per test binary; every test gets its own key prefix on it.
var server struct {
	once sync.Once
	conn Conn
	err  error
}

// Conn is how to reach the shared bucket.
type Conn struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

// Handle is one test's key prefix in the shared bucket.
type Handle struct {
	Conn
	Prefix string
}

// New returns a handle with a fresh key prefix; without TEST_S3_ENDPOINT it skips if no container starts, and fails under CI.
func New(t *testing.T) *Handle {
	t.Helper()
	conn := Conn{
		Endpoint: os.Getenv(envEndpoint), Region: os.Getenv(envRegion), Bucket: os.Getenv(envBucket),
		AccessKeyID: os.Getenv(envKeyID), SecretAccessKey: os.Getenv(envSecret),
	}
	if conn.Endpoint == "" {
		server.once.Do(func() { server.conn, server.err = startContainer(t) })
		if server.err != nil {
			if os.Getenv(envCI) != "" {
				t.Fatalf("s3test: no S3 under CI: set %s or provide a container runtime: %v", envEndpoint, server.err)
			}
			t.Skipf("s3test: cannot start Garage (set %s or start docker/podman): %v", envEndpoint, server.err)
		}
		conn = server.conn
	}
	return &Handle{Conn: conn, Prefix: uniquePrefix(t)}
}

// Client opens an SDK client on the handle's bucket, and deletes every object under the handle's prefix when the test ends.
func (h *Handle) Client(t *testing.T) *awss3.Client {
	t.Helper()
	c := newClient(h.Conn)
	t.Cleanup(func() {
		ctx := context.Background()
		pages := awss3.NewListObjectsV2Paginator(c, &awss3.ListObjectsV2Input{Bucket: aws.String(h.Bucket), Prefix: aws.String(h.Prefix)})
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				t.Errorf("s3test: list %s: %v", h.Prefix, err)
				return
			}
			for _, o := range page.Contents {
				if _, err := c.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(h.Bucket), Key: o.Key}); err != nil {
					t.Errorf("s3test: leaked %s: %v", aws.ToString(o.Key), err)
				}
			}
		}
	})
	return c
}

func newClient(c Conn) *awss3.Client {
	return awss3.New(awss3.Options{
		Region:                     c.Region,
		BaseEndpoint:               aws.String(c.Endpoint),
		UsePathStyle:               true,
		Credentials:                credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}

// NOTE: Ryuk reaps the container when the binary exits; with Ryuk off (podman) the label sweep removes it on a later run.
func startContainer(t *testing.T) (Conn, error) {
	t.Helper()
	sweepStale(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("3900/tcp", "3903/tcp"),
		testcontainers.WithLabels(map[string]string{labelOwner: "true"}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader: strings.NewReader(garageToml), ContainerFilePath: "/etc/garage.toml", FileMode: 0o644,
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3900/tcp").WithStartupTimeout(time.Minute),
			wait.ForListeningPort("3903/tcp").WithStartupTimeout(time.Minute),
		),
	)
	if err != nil {
		return Conn{}, fmt.Errorf("start %s: %w", image, err)
	}
	s3URL, err := c.PortEndpoint(ctx, "3900/tcp", "http")
	if err == nil {
		var adminURL string
		if adminURL, err = c.PortEndpoint(ctx, "3903/tcp", "http"); err == nil {
			conn := Conn{Endpoint: s3URL, Region: region, Bucket: bucket, AccessKeyID: keyID, SecretAccessKey: keySecret}
			if err = initGarage(ctx, adminURL, conn); err == nil {
				return conn, nil
			}
		}
	}
	_ = c.Terminate(context.Background())
	return Conn{}, err
}

func initGarage(ctx context.Context, adminURL string, conn Conn) error {
	var status struct {
		LayoutVersion int `json:"layoutVersion"`
		Nodes         []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	for {
		err := admin(ctx, adminURL, http.MethodGet, "/v2/GetClusterStatus", nil, &status)
		if err == nil && len(status.Nodes) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("garage admin API: %w", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
	role := map[string]any{"id": status.Nodes[0].ID, "zone": "dc1", "capacity": 1_000_000_000, "tags": []string{}}
	roles := map[string]any{"roles": []map[string]any{role}}
	var created struct {
		ID string `json:"id"`
	}
	steps := []struct {
		path string
		body any
		out  any
	}{
		{"/v2/UpdateClusterLayout", roles, nil},
		{"/v2/ApplyClusterLayout", map[string]int{"version": status.LayoutVersion + 1}, nil},
		{"/v2/CreateBucket", map[string]string{"globalAlias": conn.Bucket}, &created},
		{"/v2/ImportKey", map[string]string{
			"accessKeyId": conn.AccessKeyID, "secretAccessKey": conn.SecretAccessKey, "name": "oagent-test",
		}, nil},
	}
	for _, s := range steps {
		if err := admin(ctx, adminURL, http.MethodPost, s.path, s.body, s.out); err != nil {
			return err
		}
	}
	allow := map[string]any{
		"bucketId": created.ID, "accessKeyId": conn.AccessKeyID,
		"permissions": map[string]bool{"read": true, "write": true, "owner": true},
	}
	if err := admin(ctx, adminURL, http.MethodPost, "/v2/AllowBucketKey", allow, nil); err != nil {
		return err
	}
	_, err := newClient(conn).PutBucketCors(ctx, &awss3.PutBucketCorsInput{
		Bucket: aws.String(conn.Bucket),
		CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
			AllowedOrigins: []string{corsOrigin}, AllowedMethods: []string{"PUT", "GET", "HEAD"},
			AllowedHeaders: []string{"content-type"}, ExposeHeaders: []string{"etag"}, MaxAgeSeconds: aws.Int32(3600),
		}}},
	})
	if err != nil {
		return fmt.Errorf("garage bucket cors: %w", err)
	}
	return nil
}

func admin(ctx context.Context, base, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("garage %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func sweepStale(t *testing.T) {
	t.Helper()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return
	}
	defer func() { _ = provider.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cli := provider.Client()
	found, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", labelOwner+"=true"),
	})
	if err != nil {
		t.Logf("s3test: sweep: list: %v", err)
		return
	}
	cutoff := time.Now().Add(-staleAfter).Unix()
	for _, ctr := range found.Items {
		if ctr.Created > cutoff {
			continue
		}
		if _, err := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
			t.Logf("s3test: sweep: remove %s: %v", ctr.ID, err)
		}
	}
}

func uniquePrefix(t *testing.T) string {
	t.Helper()
	id, err := nanoid.New(prefixLen)
	if err != nil {
		t.Fatalf("s3test: nanoid: %v", err)
	}
	return "oagenttest/" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(id)) + "/"
}
