// Package s3 opens and watches the one S3-compatible bucket (R2, Garage, AWS) the storage driver uses.
package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"altalune.id/oagent/httpclient"
)

// Client is the open connection: S3 for server calls, Presign for URLs a browser uses, and the bucket name.
type Client struct {
	S3      *awss3.Client
	Presign *awss3.PresignClient
	Bucket  string
	http    *http.Client
}

// Open builds the clients and checks the bucket once within Timeout. SECURITY: errors name the host and bucket, never the keys.
func Open(ctx context.Context, c Config) (*Client, error) {
	endpoint, err := endpointURL(c.Endpoint, "s3.endpoint")
	if err != nil {
		return nil, err
	}
	browser := endpoint
	if c.BrowserEndpoint != "" {
		if browser, err = endpointURL(c.BrowserEndpoint, "s3.browserEndpoint"); err != nil {
			return nil, err
		}
	}
	// NOTE: the endpoint is trusted operator config, often on a private network, so the SSRF filter is off.
	hc := httpclient.New(httpclient.WithTimeout(c.Timeout), httpclient.WithAllowPrivateHosts(true))
	client := awss3.New(awss3.Options{
		Region:                     c.Region,
		BaseEndpoint:               aws.String(endpoint),
		UsePathStyle:               c.UsePathStyle,
		Credentials:                credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, ""),
		HTTPClient:                 hc,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	presign := awss3.NewPresignClient(client, func(po *awss3.PresignOptions) {
		po.ClientOptions = append(po.ClientOptions, func(o *awss3.Options) { o.BaseEndpoint = aws.String(browser) })
	})
	cl := &Client{S3: client, Presign: presign, Bucket: c.Bucket, http: hc}

	pingCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if err := cl.headBucket(pingCtx); err != nil {
		hc.CloseIdleConnections()
		return nil, fmt.Errorf("s3: cannot reach bucket %q at %s (s3.endpoint): %w", c.Bucket, hostOf(endpoint), err)
	}
	return cl, nil
}

// Close drops the idle connections; the SDK client holds nothing else.
func (c *Client) Close() error {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
	return nil
}

func (c *Client) headBucket(ctx context.Context) error {
	_, err := c.S3.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: aws.String(c.Bucket)})
	return err
}

// SECURITY: the value may carry credentials, so no error repeats it or the parser's message.
func endpointURL(raw, key string) (string, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil, u.Scheme != "http" && u.Scheme != "https", u.Host == "":
		return "", fmt.Errorf("s3: %s must be an http:// or https:// URL with a host", key)
	case u.User != nil, u.RawQuery != "", u.Fragment != "":
		return "", fmt.Errorf("s3: %s must not carry credentials, a query or a fragment", key)
	}
	return strings.TrimSuffix(raw, "/"), nil
}

func hostOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "the endpoint"
	}
	return u.Host
}
